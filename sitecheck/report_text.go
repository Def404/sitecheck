package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

func printResult(w io.Writer, result Result) error {
	var err error
	printf := func(format string, args ...any) {
		if err != nil {
			return
		}
		_, err = fmt.Fprintf(w, format, args...)
	}

	if result.StatusCode == 0 {
		printf("\tStatus: -\n")
	} else {
		printf("\tStatus: %d\n", result.StatusCode)
	}

	state := result.State()
	printf("\tState: %s\n", state)

	if u, perr := parseURL(result.URL); perr == nil && u.Scheme == "https" {
		status := tlsStatus(result.TLSExpiresAt, result.TLSErr)

		printf("\tTLS: %s\n", status)

		switch status {
		case "ERROR":
			printf("\tTLS error: %s\n", result.TLSErr)
		case "EXPIRED":
			printf("\tCertificate expired: %s\n", result.TLSExpiresAt.Format(time.DateOnly))
		case "UNKNOWN":
			printf("\tCertificate information unavailable\n")
		case "OK", "WARNING":
			printf("\tCertificate expires: %s\n", result.TLSExpiresAt.Format(time.DateOnly))
			printf("\tDays remaining: %d\n", daysUntilExpiration(result.TLSExpiresAt))
		}
	}

	printf("\tDuration: %d ms\n", result.Duration.Milliseconds())

	if result.Err != nil {
		printf("\tError: %s\n", result.Err)
	}

	if result.Reason != "" {
		printf("\tReason: %s\n", result.Reason)
	}

	if len(result.IPs) != 0 {
		printf("\tIP addresses:\n")
		for _, ip := range result.IPs {
			printf("\t\t- %s\n", ip)
		}
	}

	if result.DNSErr != nil {
		printf("\tDNS Error: %v\n", result.DNSErr)
	}

	return err
}

func printSystemInfo(w io.Writer, system SystemInfo) error {
	_, err := fmt.Fprintf(w, "[СИСТЕМА]\n")
	if err != nil {
		return err
	}

	if _, err = fmt.Fprintf(w, "  OS: %s\n", system.OS); err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "  User: %s@%s\n", system.Username, system.Hostname)
	return err
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

	if err = printSystemInfo(w, report.System); err != nil {
		return err
	}

	println()

	for _, res := range report.Results {
		if err != nil {
			return err
		}
		printf("Checking: %s\n", res.URL)
		if err != nil {
			return err
		}
		if err = printResult(w, res); err != nil {
			return err
		}
		println()
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
