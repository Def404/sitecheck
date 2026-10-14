package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

type Result struct {
	URL          string
	StatusCode   int
	Up           bool
	Duration     time.Duration
	Err          error
	Reason       string
	IPs          []string
	DNSErr       error
	TLSExpiresAt time.Time
	TLSErr       error
}

type JSONResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Up         bool   `json:"up"`
	DurationMS int64  `json:"duration_ms"`
	Reason     string `json:"reason"`
}

func main() {

	err := run(os.Args[1:])

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {

	path, jsonOutput, outputFile, err := parseArgs(args)

	if err != nil {
		return err
	}

	urls, err := loadURLs(path)
	if err != nil {
		return fmt.Errorf("load urls from %s: %w", path, err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // не идти по redirect, вернуть ответ как есть
		},
	}

	upCount := 0
	downCount := 0

	results := make([]Result, 0, len(urls))

	for _, url := range urls {

		result := check(client, url, resolveIPs)
		results = append(results, result)

		if result.Up {
			upCount++
		} else {
			downCount++
		}
	}

	switch {
	case !jsonOutput:
		for _, res := range results {
			fmt.Printf("Checking: %s\n", res.URL)
			printResult(res)
			fmt.Println()
		}

		fmt.Printf("Summary: %d UP / %d DOWN\n", upCount, downCount)
	case outputFile == "":
		err := printJSONResults(results)
		if err != nil {
			return err
		}
	default:
		err := saveJSONResults(outputFile, results)
		if err != nil {
			return fmt.Errorf("save JSON report to %s: %w", outputFile, err)
		}
	}

	return nil
}

func parseArgs(args []string) (filename string, jsonOutput bool, outputFile string, err error) {
	fs := flag.NewFlagSet("sitecheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	json := fs.Bool("json", false, "output results as JSON")
	output := fs.String("output", "", "write JSON report to this file")

	if err := fs.Parse(args); err != nil {
		return "", false, "", err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		if len(rest) == 0 {
			return "", false, "", errors.New("filename is required")
		}
		return "", false, "", errors.New("too many arguments")
	}
	if rest[0] == "" {
		return "", false, "", errors.New("filename is required")
	}
	if *output != "" && !*json {
		return "", false, "", errors.New("--output requires --json")
	}

	return rest[0], *json, *output, nil
}

func loadURLs(path string) ([]string, error) {
	f, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer f.Close()

	var lines []string

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func check(client *http.Client, url string, resolve func(string) ([]string, error)) Result {

	result := Result{
		URL: url,
	}

	start := time.Now()

	u, err := parseURL(url)
	if err != nil {
		result.Err = err
		result.Reason = "invalid URL"
		return result
	}

	resp, err := client.Get(url)
	result.Duration = time.Since(start)

	if err != nil {
		result.Err = err
		result.Reason = errorReason(err)
	} else {
		defer resp.Body.Close()

		result.StatusCode = resp.StatusCode
		result.Up = isUp(resp.StatusCode)
		result.Reason = statusReason(resp.StatusCode)
	}

	result.IPs, result.DNSErr = resolve(u.Hostname())

	if u.Scheme == "https" {
		port := u.Port()
		if port == "" {
			port = "443"
		}
		result.TLSExpiresAt, result.TLSErr = getTLSExpiration(u.Hostname(), port)
	}

	return result
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse URL %q: %w", raw, err)
	}

	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL: %q", raw)
	}

	return u, nil
}

func isUp(statusCode int) bool {
	return statusCode >= 200 && statusCode < 400
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error

	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

func isDNSError(err error) bool {
	if err == nil {
		return false
	}

	var dnsErr *net.DNSError

	if errors.As(err, &dnsErr) {
		return true
	}

	return false
}

func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	var unknownAuth x509.UnknownAuthorityError
	if errors.As(err, &unknownAuth) {
		return true
	}

	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		return true
	}

	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}

	return false
}

func isExpiredCertificate(err error) bool {
	if err == nil {
		return false
	}

	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		return true
	}

	return false
}

func resolveIPs(host string) ([]string, error) {
	ips, err := net.LookupIP(host)

	if err != nil {
		return nil, fmt.Errorf("resolve IPs for %q: %w", host, err)
	}

	addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, ip.String())
	}

	return addrs, nil
}

func getTLSExpiration(host string, port string) (time.Time, error) {

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		net.JoinHostPort(host, port),
		&tls.Config{
			ServerName: host,
		},
	)

	if err != nil {
		return time.Time{}, fmt.Errorf("TLS connection to %s: %w", host, err)
	}

	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates

	if len(certs) == 0 {
		return time.Time{}, fmt.Errorf("TLS connection to %s: %w", host, err)
	}

	return certs[0].NotAfter, nil
}

func daysUntilExpiration(expiresAt time.Time) int {
	remaining := time.Until(expiresAt)

	days := int(remaining / (24 * time.Hour))

	if days < 0 {
		return 0
	}

	return days
}

func statusReason(statusCode int) string {
	if isUp(statusCode) {
		return ""
	}

	return fmt.Sprintf("HTTP %d", statusCode)
}

func errorReason(err error) string {

	if err == nil {
		return ""
	}

	if isTimeout(err) {
		return "timeout"
	}

	if isDNSError(err) {
		return "DNS"
	}

	if isConnectionRefused(err) {
		return "connection refused"
	}

	if isTLSError(err) {
		return "TLS"
	}

	return "request failed"
}

func tlsStatus(expiresAt time.Time, err error) string {
	if err != nil && isExpiredCertificate(err) {
		return "EXPIRED"
	}

	if err != nil {
		return "ERROR"
	}

	if expiresAt.IsZero() {
		return "UNKNOWN"
	}

	if time.Until(expiresAt) < 7*24*time.Hour {
		return "WARNING"
	}

	return "OK"
}

func printResult(result Result) {
	if result.StatusCode == 0 {
		fmt.Println("\tStatus: -")
	} else {
		fmt.Printf("\tStatus: %d\n", result.StatusCode)
	}

	if result.Up {
		fmt.Printf("\tState: UP\n")
	} else {
		fmt.Printf("\tState: DOWN\n")
	}

	if u, err := parseURL(result.URL); err == nil && u.Scheme == "https" {
		status := tlsStatus(result.TLSExpiresAt, result.TLSErr)

		fmt.Printf("\tTLS: %s\n", status)

		switch status {
		case "ERROR", "EXPIRED":
			fmt.Printf("\tTLS error: %s\n", result.TLSErr)
		case "UNKNOWN":
			fmt.Printf("\tCertificate information unavailable\n")
		case "OK", "WARNING":
			fmt.Printf("\tCertificate expires: %s\n", result.TLSExpiresAt.Format(time.DateOnly))
			fmt.Printf("\tDays remaining: %d\n", daysUntilExpiration(result.TLSExpiresAt))

		}
	}

	fmt.Printf("\tDuration: %d ms\n", result.Duration.Milliseconds())

	if result.Err != nil {
		fmt.Printf("\tError: %s\n", result.Err)
	}

	if result.Reason != "" {
		fmt.Printf("\tReason: %s\n", result.Reason)
	}

	if len(result.IPs) != 0 {

		fmt.Printf("\tIP addresses:\n")
		for _, ip := range result.IPs {
			fmt.Printf("\t\t- %s\n", ip)
		}
	}

	if result.DNSErr != nil {
		fmt.Printf("\tDNS Error: %v\n", result.DNSErr)
	}
}

func marshalResults(results []Result) ([]byte, error) {
	jsonResults := make([]JSONResult, 0, len(results))
	for _, result := range results {
		jsonResults = append(jsonResults, toJSONResult(result))
	}

	return json.MarshalIndent(jsonResults, "", "  ")
}

func toJSONResult(result Result) JSONResult {
	return JSONResult{
		URL:        result.URL,
		StatusCode: result.StatusCode,
		Up:         result.Up,
		DurationMS: result.Duration.Milliseconds(),
		Reason:     result.Reason,
	}
}

func printJSON(result Result) error {
	data, err := json.MarshalIndent(toJSONResult(result), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func printJSONResults(results []Result) error {
	data, err := marshalResults(results)
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func saveJSON(filename string, data []byte) error {
	return os.WriteFile(filename, data, 0o644)
}

func saveJSONResults(filename string, results []Result) error {
	data, err := marshalResults(results)
	if err != nil {
		return err
	}

	return saveJSON(filename, data)
}
