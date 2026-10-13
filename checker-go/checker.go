package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Site struct {
	URL  string
	Host string
}

type PingResult struct {
	OK     bool
	AVGRtt float64
	Lost   int
	Error  string
}

func (p PingResult) StatusStr() string {
	if p.OK {
		return fmt.Sprintf("ping OK %6.1f ms", p.AVGRtt*1000)
	}
	if p.Error != "" {
		return "ping FAIL: " + p.Error
	}
	return "ping FAIL: timeout"
}

type CheckResult struct {
	URL         string
	Host        string
	OK          bool
	HTTPStatus  string
	HTTPElapsed float64
	Ping        *PingResult
	IP          string
	Headers     map[string]string
	ErrBody     string
}

var urlSchemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if urlSchemeRe.MatchString(raw) {
		return raw
	}
	return "https://" + raw
}

func siteHost(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	return parsed.Host
}

func loadSites(argsURLs []string, filePath string) ([]Site, error) {
	var raws []string
	raws = append(raws, argsURLs...)
	if filePath != "" {
		data, err := readFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать файл %s: %w", filePath, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			raws = append(raws, line)
		}
	}

	var sites []Site
	seen := make(map[string]bool)
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		u := normalizeURL(raw)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		sites = append(sites, Site{URL: u, Host: siteHost(u)})
	}
	return sites, nil
}

func checkSite(site Site, timeout time.Duration, doPing bool) CheckResult {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("слишком много редиректов (> 5)")
			}
			return nil
		},
	}

	type httpResult struct {
		ok        bool
		status    string
		elapsed   float64
		headers   map[string]string
		errBody   string
	}

	var wg sync.WaitGroup
	var hr httpResult
	var pr *PingResult

	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		req, err := http.NewRequest("GET", site.URL, nil)
		if err != nil {
			hr = httpResult{ok: false, status: "error: " + err.Error(), elapsed: 0}
			return
		}
		req.Header.Set("User-Agent", "checker/1.0 (+go-net-http)")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

		resp, err := client.Do(req)
		elapsed := time.Since(start).Seconds()
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline") {
				hr = httpResult{ok: false, status: "timeout", elapsed: elapsed}
			} else {
				hr = httpResult{ok: false, status: "error: " + errStr, elapsed: elapsed}
			}
			return
		}
		defer resp.Body.Close()

		hdrs := make(map[string]string)
		for _, h := range []string{"Server", "X-Powered-By", "Via", "Content-Type"} {
			if v := resp.Header.Get(h); v != "" {
				hdrs[h] = v
			}
		}
		// TLS info
		if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
			cert := resp.TLS.PeerCertificates[0]
			if len(cert.DNSNames) > 0 {
				hdrs["TLS"] = cert.DNSNames[0]
			} else if cert.Subject.CommonName != "" {
				hdrs["TLS"] = cert.Subject.CommonName
			}
		}

		ok := resp.StatusCode >= 200 && resp.StatusCode < 400
		status := fmt.Sprintf("%d", resp.StatusCode)

		// Read error body for 4xx/5xx
		var body string
		if !ok {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
			body = strings.TrimSpace(string(data))
			if len(body) > 200 {
				body = body[:200]
			}
		}

		hr = httpResult{ok: ok, status: status, elapsed: elapsed, headers: hdrs, errBody: body}
	}()

	if doPing {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pr = pingHost(site.Host, 3, 2*time.Second)
		}()
	}

	wg.Wait()

	// Resolve IP
	ip := ""
	addrs, err := net.LookupHost(hostWithoutPort(site.Host))
	if err == nil && len(addrs) > 0 {
		// Prefer IPv4
		for _, a := range addrs {
			if ip4 := net.ParseIP(a); ip4 != nil && ip4.To4() != nil {
				ip = a
				break
			}
		}
		if ip == "" {
			ip = addrs[0]
		}
	}

	ok := hr.ok
	if doPing && pr != nil && !pr.OK {
		// Ping doesn't affect UP/DOWN (same as Python version)
		_ = pr
	}

	return CheckResult{
		URL:         site.URL,
		Host:        site.Host,
		OK:          ok,
		HTTPStatus:  hr.status,
		HTTPElapsed: hr.elapsed,
		Ping:        pr,
		IP:          ip,
		Headers:     hr.headers,
		ErrBody:     hr.errBody,
	}
}

func runCheckRound(sites []Site, timeout float64, doPing bool) []CheckResult {
	results := make([]CheckResult, len(sites))
	var wg sync.WaitGroup
	for i, site := range sites {
		wg.Add(1)
		go func(idx int, s Site) {
			defer wg.Done()
			results[idx] = checkSite(s, time.Duration(timeout*float64(time.Second)), doPing)
		}(i, site)
	}
	wg.Wait()
	return results
}

func hostWithoutPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// --- ICMP Ping ---

func pingHost(host string, count int, timeout time.Duration) *PingResult {
	addr := hostWithoutPort(host)
	ips, err := net.LookupHost(addr)
	if err != nil {
		return &PingResult{OK: false, Error: "DNS: " + err.Error()}
	}
	if len(ips) == 0 {
		return &PingResult{OK: false, Error: "DNS: no address"}
	}
	return doICMPPing(ips[0], count, timeout)
}
