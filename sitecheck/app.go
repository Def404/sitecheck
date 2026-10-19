// Package main — sitecheck: однократная проверка доступности списка сайтов.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	if opts.Help {
		return printHelp(os.Stdout)
	}

	if opts.Version {
		return printVersion(os.Stdout)
	}

	urls, err := loadURLs(opts.Path)
	if err != nil {
		return fmt.Errorf("load urls from %s: %w", opts.Path, err)
	}

	timeout, err := getTimeout()
	if err != nil {
		return err
	}

	client := newHTTPClient(opts)

	sysInfo, err := getSystemInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not get system info: %s\n", err)
	}

	netInfo := getNetworkInfo(ctx)

	results := make([]Result, len(urls))

	sem := make(chan struct{}, opts.Concurrency)

	start := time.Now()

	var wg sync.WaitGroup

	// Прогресс-строка пишется в stdout; при JSON или --text-output
	// подавляется (io.Discard), чтобы не смешивать \r с JSON/файлом.
	var progressOut io.Writer
	if opts.JSONOutput || opts.JSONReport || opts.TextOutput != "" {
		progressOut = io.Discard
	} else {
		progressOut = os.Stdout
	}

	progress := newProgress(progressOut, len(urls), useColor)

	wg.Add(len(urls))

	for i, url := range urls {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			var result Result
			attempts := 0
			for attempt := 0; attempt <= opts.Retries; attempt++ {
				result = check(ctx, client, url, timeout, resolveIPs)
				attempts++
				if !result.Retryable {
					break
				}
				if attempt < opts.Retries {
					select {
					case <-time.After(200 * time.Millisecond):
					case <-ctx.Done():
					}
					if ctx.Err() != nil {
						break
					}
				}
			}
			result.Attempts = attempts
			results[i] = result
			progress.tick()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Без сигнала ждём все проверки: иначе tick() дописывает полосу
	// через \r поверх уже печатаемого отчёта. После Ctrl+C — не дольше 5 с,
	// затем частичный отчёт.
	select {
	case <-done:
	case <-ctx.Done():
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}

	progress.finish()

	checkDuration := time.Since(start)

	interrupted := ctx.Err() != nil
	if interrupted {
		for i := range results {
			if results[i].Attempts == 0 {
				results[i].URL = urls[i]
				results[i].Skipped = true
			}
		}
	}

	report := buildReport(sysInfo, netInfo, results, checkDuration)

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

	if interrupted {
		return fmt.Errorf("interrupted: %w", errInterrupted)
	}

	for _, result := range results {
		if !result.Up {
			return ErrSitesDown
		}
	}

	return nil
}

func check(ctx context.Context, client *http.Client, url string, timeout time.Duration, resolve func(context.Context, string) ([]string, error)) (result Result) {
	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	ctx, cancel := context.WithTimeout(ctx, timeout)
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
			result.Retryable = isRetryableError(err)
		} else {
			defer func() {
				if cerr := resp.Body.Close(); cerr != nil {
					result.Err = cerr
				}
			}()

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

// newHTTPClient — клиент для проверки сайтов: redirect не следует
// (CheckRedirect → http.ErrUseLastResponse), таймаут на запрос задаётся
// отдельно через context в check(). Настройки TLS/Transport — будущие
// расширения (TODOS #39).
func newHTTPClient(opts Options) *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
