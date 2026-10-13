package main

import (
	"bufio"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

type Result struct {
	URL        string
	StatusCode int
	Up         bool
	Duration   time.Duration
	Err        error
	Reason     string
}

func main() {

	err := run(os.Args[1:])

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {

	if len(args) != 1 {
		return fmt.Errorf("path is required")
	}

	urls, err := loadURLs(args[0])
	if err != nil {
		return fmt.Errorf("load urls from %s: %w", args[0], err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // не идти по redirect, вернуть ответ как есть
		},
	}

	upCount := 0
	downCount := 0

	for _, url := range urls {
		fmt.Printf("Checking: %s\n", url)

		result := check(client, url)

		if result.Up {
			upCount++
		} else {
			downCount++
		}

		printResult(result)

		fmt.Println()
	}

	fmt.Printf("Summary: %d UP / %d DOWN\n", upCount, downCount)

	return nil
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

func check(client *http.Client, url string) Result {
	start := time.Now()

	if _, err := parseURL(url); err != nil {
		return Result{
			URL:    url,
			Err:    err,
			Reason: "invalid URL",
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		d := time.Since(start)

		return Result{
			URL:      url,
			Duration: d,
			Err:      err,
			Reason:   errorReason(err),
		}
	}
	defer resp.Body.Close()

	d := time.Since(start)

	return Result{
		URL:        url,
		StatusCode: resp.StatusCode,
		Up:         isUp(resp.StatusCode),
		Duration:   d,
		Reason:     statusReason(resp.StatusCode),
	}
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

func hostname(raw string) (string, error) {
	u, err := parseURL(raw)
	if err != nil {
		return "", err
	}

	return u.Hostname(), nil
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

	fmt.Printf("\tDuration: %d ms\n", result.Duration.Milliseconds())

	if result.Err != nil {
		fmt.Printf("\tError: %s\n", result.Err)
	}

	if result.Reason != "" {
		fmt.Printf("\tReason: %s\n", result.Reason)
	}
}
