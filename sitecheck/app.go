package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

func run(args []string) error {
	ctx := context.Background()

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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	sysInfo, err := getSystemInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get system info: %s\n", err)
	}

	netInfo, err := getNetworkInfo(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get network info: %s\n", err)
	}

	results := make([]Result, 0, len(urls))

	for _, url := range urls {
		result := check(client, url, timeout, resolveIPs)
		results = append(results, result)
	}

	report := buildReport(sysInfo, netInfo, results)

	switch {
	case opts.JSONReport:
		data, err := marshalReport(report)
		if err != nil {
			return err
		}
		if opts.OutputFile == "" {
			fmt.Println(string(data))
		} else if err = saveJSON(opts.OutputFile, data); err != nil {
			return fmt.Errorf("save JSON report to %s: %w", opts.OutputFile, err)
		}
	case opts.JSONOutput:
		if opts.OutputFile == "" {
			err := printJSONResults(results)
			if err != nil {
				return err
			}
		} else if err := saveJSONResults(opts.OutputFile, results); err != nil {
			return fmt.Errorf("save JSON report to %s: %w", opts.OutputFile, err)
		}
	default:
		if opts.TextOutput == "" {
			if err := printReport(os.Stdout, report); err != nil {
				return err
			}
		} else if err := writeReportToBoth(report, opts.TextOutput); err != nil {
			return err
		}
	}

	for _, result := range results {
		if !result.Up {
			return ErrSitesDown
		}
	}

	return nil
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
