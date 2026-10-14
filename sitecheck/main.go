package main

import (
	"bufio"
	"bytes"
	"context"
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
	"sort"
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

func (r Result) State() string {
	if r.Up {
		return "UP"
	}

	return "DOWN"
}

type JSONResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Up         bool   `json:"up"`
	DurationMS int64  `json:"duration_ms"`
	Reason     string `json:"reason"`
}

type Report struct {
	Results   []Result
	Total     int
	UpCount   int
	DownCount int
	Reasons   map[string]int
}

type JSONReport struct {
	Total   int            `json:"total"`
	Up      int            `json:"up"`
	Down    int            `json:"down"`
	Reasons map[string]int `json:"reasons"`
	Results []JSONResult   `json:"results"`
}

type Options struct {
	Path       string
	JSONOutput bool
	JSONReport bool
	OutputFile string
	TextOutput string
}

func main() {

	err := run(os.Args[1:])

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {

	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	urls, err := loadURLs(opts.Path)
	if err != nil {
		return fmt.Errorf("load urls from %s: %w", opts.Path, err)
	}

	timeout, err := getTimeout()
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // не идти по redirect, вернуть ответ как есть
		},
	}

	results := make([]Result, 0, len(urls))

	for _, url := range urls {
		result := check(client, url, timeout, resolveIPs)
		results = append(results, result)
	}

	switch {
	case opts.JSONReport:
		data, err := marshalReport(buildReport(results))
		if err != nil {
			return err
		}
		if opts.OutputFile == "" {
			fmt.Println(string(data))
			return nil
		}
		err = saveJSON(opts.OutputFile, data)
		if err != nil {
			return fmt.Errorf("save JSON report to %s: %w", opts.OutputFile, err)
		}
	case opts.JSONOutput:
		if opts.OutputFile == "" {
			err := printJSONResults(results)
			if err != nil {
				return err
			}
			return nil
		}
		err := saveJSONResults(opts.OutputFile, results)
		if err != nil {
			return fmt.Errorf("save JSON report to %s: %w", opts.OutputFile, err)
		}
	default:
		report := buildReport(results)

		for _, res := range results {
			fmt.Printf("Checking: %s\n", res.URL)
			printResult(res)
			fmt.Println()
		}

		if opts.TextOutput == "" {
			if err := printReport(os.Stdout, report); err != nil {
				return err
			}
			return nil
		}
		if err := writeReportToBoth(report, opts.TextOutput); err != nil {
			return err
		}
	}

	return nil
}

func parseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("sitecheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	json := fs.Bool("json", false, "output results as JSON")
	jsonReport := fs.Bool("json-report", false, "output a JSON report")
	output := fs.String("output", "", "write JSON report to this file")
	textOutput := fs.String("text-output", "", "write the text summary to this file")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		if len(rest) == 0 {
			return Options{}, errors.New("filename is required")
		}
		return Options{}, errors.New("too many arguments")
	}
	if rest[0] == "" {
		return Options{}, errors.New("filename is required")
	}
	if *output != "" && !*json && !*jsonReport {
		return Options{}, errors.New("--output requires --json or --json-report")
	}
	if *json && *jsonReport {
		return Options{}, errors.New("--json and --json-report cannot be used together")
	}
	if *textOutput != "" && (*json || *jsonReport) {
		return Options{}, errors.New("--text-output cannot be used with --json or --json-report")
	}

	return Options{
		Path:       rest[0],
		JSONOutput: *json,
		JSONReport: *jsonReport,
		OutputFile: *output,
		TextOutput: *textOutput,
	}, nil
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

func check(client *http.Client, url string, timeout time.Duration, resolve func(context.Context, string) ([]string, error)) (result Result) {
	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result = Result{
		URL: url,
	}

	u, err := parseURL(url)
	if err != nil {
		result.Err = err
		result.Reason = "invalid URL"
		return result
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		result.Err = err
		result.Reason = errorReason(err)
	} else {
		resp, err := client.Do(req)
		if err != nil {
			result.Err = err
			result.Reason = errorReason(err)
		} else {
			defer resp.Body.Close()

			result.StatusCode = resp.StatusCode
			result.Up = isUp(resp.StatusCode)
			result.Reason = statusReason(resp.StatusCode)
		}
	}

	result.IPs, result.DNSErr = resolve(ctx, u.Hostname())

	if u.Scheme == "https" {
		port := u.Port()
		if port == "" {
			port = "443"
		}
		result.TLSExpiresAt, result.TLSErr = getTLSExpiration(ctx, u.Hostname(), port)
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

	if errors.Is(err, context.DeadlineExceeded) {
		return true
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

func resolveIPs(ctx context.Context, host string) ([]string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve IPs for %q: %w", host, err)
	}

	addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, ip.IP.String())
	}

	return addrs, nil
}

func getTLSExpiration(ctx context.Context, host string, port string) (time.Time, error) {
	dialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName: host,
		},
	}

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return time.Time{}, fmt.Errorf("TLS connection to %s: %w", host, err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return time.Time{}, fmt.Errorf("TLS connection to %s: unexpected connection type %T", host, conn)
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, fmt.Errorf("TLS connection to %s: no peer certificates", host)
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

	state := result.State()
	fmt.Printf("\tState: %s\n", state)

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

func toJSONReport(report Report) JSONReport {
	results := make([]JSONResult, 0, len(report.Results))
	for _, result := range report.Results {
		results = append(results, toJSONResult(result))
	}

	return JSONReport{
		Total:   report.Total,
		Up:      report.UpCount,
		Down:    report.DownCount,
		Reasons: report.Reasons,
		Results: results,
	}
}

func marshalReport(report Report) ([]byte, error) {
	return json.MarshalIndent(toJSONReport(report), "", "    ")
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

func printReport(w io.Writer, report Report) error {
	var err error
	println := func(args ...any) {
		if err != nil {
			return
		}
		_, err = fmt.Fprintln(w, args...)
	}
	printf := func(format string, args ...any) {
		if err != nil {
			return
		}
		_, err = fmt.Fprintf(w, format, args...)
	}

	println("Summary")
	println("-------")
	printf("Total:  %d\n", report.Total)
	printf("UP:     %d\n", report.UpCount)
	printf("DOWN:   %d\n", report.DownCount)

	if err != nil || len(report.Reasons) == 0 {
		return err
	}

	reasons := make([]string, 0, len(report.Reasons))
	for reason := range report.Reasons {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)

	println()
	println("Reasons:")
	for _, reason := range reasons {
		printf("  %s: %d\n", reason, report.Reasons[reason])
	}
	return err
}

func reportToString(report Report) (string, error) {
	var buf bytes.Buffer
	if err := printReport(&buf, report); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeReportToBoth(report Report, path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report file %s: %w", path, err)
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil {
			err = errors.Join(err, fmt.Errorf("close report file %s: %w", path, cerr))
		}
	}()

	err = printReport(io.MultiWriter(os.Stdout, f), report)
	if err != nil {
		return fmt.Errorf("write report to %s: %w", path, err)
	}
	return nil
}

func getTimeout() (time.Duration, error) {
	value := os.Getenv("SITECHECK_TIMEOUT")

	if value == "" {
		return 5 * time.Second, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid SITECHECK_TIMEOUT %q: %w", value, err)
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("SITECHECK_TIMEOUT must be positive, got %s", timeout)
	}

	return timeout, nil
}

func countReasons(results []Result) map[string]int {
	counts := make(map[string]int)
	for _, result := range results {
		if result.Reason == "" {
			continue
		}
		counts[result.Reason]++
	}
	return counts
}

func buildReport(results []Result) Report {

	total := len(results)
	upCount := 0

	for _, result := range results {
		if result.Up {
			upCount++
		}
	}

	return Report{
		Results:   results,
		Total:     total,
		UpCount:   upCount,
		DownCount: total - upCount,
		Reasons:   countReasons(results),
	}
}
