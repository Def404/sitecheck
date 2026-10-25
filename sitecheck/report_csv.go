// Package main — sitecheck: однократная проверка доступности списка сайтов.
package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// csvColumns — заголовок CSV-отчёта (TODOS #26).
var csvColumns = []string{
	"URL", "Status", "State", "Duration_ms", "Attempts",
	"Reason", "IP", "Ping_ms", "TLS_expires", "TLS_status",
}

// writeCSV записывает результаты в w в формате CSV (заголовок + по строке
// на URL). Записи экранируются стандартным csv.Writer.
func writeCSV(w io.Writer, results []Result) error {
	writer := csv.NewWriter(w)

	if err := writer.Write(csvColumns); err != nil {
		return err
	}

	for _, r := range results {
		if err := writer.Write(csvRow(r)); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

func csvRow(r Result) []string {
	state := "—"
	status := ""
	duration := "0"
	attempts := "0"
	reason := ""
	ip := ""
	ping := ""
	tlsExpires := ""
	tls := ""

	if !r.Skipped {
		state = r.State()
		if r.StatusCode > 0 {
			status = fmt.Sprintf("%d", r.StatusCode)
		}
		duration = fmt.Sprintf("%d", r.Duration.Milliseconds())
		attempts = fmt.Sprintf("%d", r.Attempts)
		reason = r.Reason
		if len(r.IPs) > 0 {
			ip = r.IPs[0]
		}
	}

	if r.Ping.Ran {
		ping = fmt.Sprintf("%.2f", r.Ping.LatencyMS)
	}

	if r.TLSErr != nil || !r.TLSExpiresAt.IsZero() {
		tls = tlsStatus(r.TLSExpiresAt, r.TLSErr)
		if !r.TLSExpiresAt.IsZero() {
			tlsExpires = r.TLSExpiresAt.UTC().Format(time.RFC3339)
		}
	}

	return []string{
		r.URL,
		status,
		state,
		duration,
		attempts,
		reason,
		ip,
		ping,
		tlsExpires,
		tls,
	}
}

// writeCSVToBoth печатает CSV в stdout и дублирует в файл path (аналог
// writeReportToBoth для текстового отчёта).
func writeCSVToBoth(results []Result, path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CSV file %s: %w", path, err)
	}
	defer func() {
		cerr := f.Close()
		if cerr != nil {
			err = errors.Join(err, fmt.Errorf("close CSV file %s: %w", path, cerr))
		}
	}()

	if err = writeCSV(io.MultiWriter(os.Stdout, f), results); err != nil {
		return fmt.Errorf("write CSV to %s: %w", path, err)
	}
	return nil
}
