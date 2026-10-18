// Package main — sitecheck: однократная проверка доступности списка сайтов.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// ─── box helpers ───────────────────────────────────────────────────────────────

func boxTop(width int) string { return "┌" + strings.Repeat("─", width) + "┐" }
func boxMid(width int) string { return "├" + strings.Repeat("─", width) + "┤" }
func boxBot(width int) string { return "└" + strings.Repeat("─", width) + "┘" }
func boxRow(width int, s string) string {
	pad := width - displayWidth(s)
	if pad < 0 {
		pad = 0
	}
	return "│ " + s + strings.Repeat(" ", pad) + " │"
}

// displayWidth returns the terminal display width of s (ANSI codes = 0).
func displayWidth(s string) int {
	inCode := false
	w := 0
	for _, r := range s {
		if r == '\x1b' {
			inCode = true
			continue
		}
		if inCode {
			if r == 'm' {
				inCode = false
			}
			continue
		}
		w++
	}
	return w
}

func printBoxSection(w io.Writer, title string, lines []string) error {
	maxW := displayWidth(title)
	for _, l := range lines {
		if dw := displayWidth(l); dw > maxW {
			maxW = dw
		}
	}
	width := maxW + 4 // 2 padding each side + 2 border

	var err error
	write := func(s string) {
		if err != nil {
			return
		}
		_, err = fmt.Fprintln(w, s)
	}

	write(boxTop(width))
	write(boxRow(width, colorize(colorBold, title)))
	write(boxMid(width))
	for _, l := range lines {
		write(boxRow(width, l))
	}
	write(boxBot(width))
	return err
}

// ─── СИСТЕМА ──────────────────────────────────────────────────────────────────

func printSystemInfo(w io.Writer, system SystemInfo) error {
	lines := []string{
		"ОС:      " + system.OS,
		"Пользов: " + system.Username + "@" + system.Hostname,
	}
	return printBoxSection(w, "СИСТЕМА", lines)
}

// ─── СЕТЬ ─────────────────────────────────────────────────────────────────────

func printNetworkInfo(w io.Writer, network NetworkInfo) error {
	var lines []string

	add := func(label, value string) {
		lines = append(lines, fmt.Sprintf("%-14s %s", label+":", value))
	}

	if network.LocalIP != "" {
		add("Локальный IP", network.LocalIP)
	}

	if network.PrimaryIPError != "" {
		add("Исходящий IP", colorize(colorRed, "недоступен ("+network.PrimaryIPError+")"))
	} else {
		add("Исходящий IP", network.PrimaryIP)
	}

	switch {
	case network.InterfacesError != "":
		add("Шлюз", colorize(colorRed, "невозможно определить ("+network.InterfacesError+")"))
	case network.LANError != "":
		add("Шлюз", colorize(colorRed, "недоступен ("+network.LANError+")"))
	default:
		add("Шлюз", network.Gateway.IP+" ("+network.Gateway.Interface+")")

		switch {
		case network.GatewayPingError != "":
			add("Ping шлюза", colorize(colorRed, "ошибка: "+network.GatewayPingError))
		case network.GatewayPing.Success:
			add("Ping шлюза", colorize(colorGreen, fmt.Sprintf("OK (%.2f ms)", network.GatewayPing.LatencyMS)))
		case network.GatewayPing.Ran:
			add("Ping шлюза", colorize(colorRed, "FAIL"))
		}
	}

	switch {
	case network.DNSError != "":
		add("DNS-серверы", colorize(colorRed, "недоступны ("+network.DNSError+")"))
	case len(network.DNSInterfaces) > 0:
		for _, dnsIf := range network.DNSInterfaces {
			add("DNS ("+dnsIf.Interface+")", strings.Join(dnsIf.Servers, ", "))
		}
	default:
		add("DNS-серверы", strings.Join(network.DNSServers, ", "))
	}

	switch {
	case network.InterfacesError != "":
		add("VPN", colorize(colorRed, "невозможно определить ("+network.InterfacesError+")"))
	case len(network.VPNInterfaces) > 0:
		parts := make([]string, 0, len(network.VPNInterfaces))
		for _, vpnIf := range network.VPNInterfaces {
			parts = append(parts, vpnIf.Name+" — "+vpnIf.IP)
		}
		add("VPN", colorize(colorGreen, strings.Join(parts, ", ")))
	default:
		add("VPN", "не активен")
	}

	switch {
	case network.LocalIPsError != "":
		add("Все IPv4", colorize(colorRed, "недоступны ("+network.LocalIPsError+")"))
	case len(network.LocalIPs) > 0:
		add("Все IPv4", strings.Join(network.LocalIPs, ", "))
	default:
		add("Все IPv4", "не обнаружены")
	}

	return printBoxSection(w, "СЕТЬ", lines)
}

// ─── РЕЗУЛЬТАТЫ (таблица) ─────────────────────────────────────────────────────

func printResultsTable(w io.Writer, results []Result) error {
	type row struct {
		url      string
		status   string
		duration string
		tls      string
		reason   string
	}

	rows := make([]row, 0, len(results))
	for _, r := range results {
		host := r.URL
		if u, err := parseURL(r.URL); err == nil {
			host = u.Host
		}

		st := r.State()
		statusStr := st
		if r.StatusCode > 0 {
			statusStr = fmt.Sprintf("%d %s", r.StatusCode, st)
		}

		tlsStr := "—"
		if u, err := parseURL(r.URL); err == nil && u.Scheme == "https" {
			tlsStr = formatTLSCell(r.TLSExpiresAt, r.TLSErr)
		}

		rows = append(rows, row{
			url:      host,
			status:   statusStr,
			duration: fmt.Sprintf("%d ms", r.Duration.Milliseconds()),
			tls:      tlsStr,
			reason:   r.Reason,
		})
	}

	headers := []string{"URL", "Статус", "Время", "TLS", "Причина"}
	cols := len(headers)

	// compute column widths
	widths := make([]int, cols)
	for i, h := range headers {
		widths[i] = displayWidth(h)
	}
	for _, r := range rows {
		vals := []string{r.url, r.status, r.duration, r.tls, r.reason}
		for i, v := range vals {
			if dw := displayWidth(v); dw > widths[i] {
				widths[i] = dw
			}
		}
	}

	var err error
	write := func(s string) {
		if err != nil {
			return
		}
		_, err = fmt.Fprintln(w, s)
	}

	// column headers
	write(tblBorderTop(widths))
	write(tblCell(widths, func(i int) string {
		return colorize(colorBold, headers[i])
	}))
	write(tblBorderMid(widths))

	// data rows
	for _, r := range rows {
		vals := []string{
			r.url,
			colorize(stateColor(extractState(r.status)), r.status),
			r.duration,
			colorize(tlsColorEx(r.tls), r.tls),
			r.reason,
		}
		write(tblCell(widths, func(i int) string { return vals[i] }))
	}

	write(tblBorderBot(widths))
	return err
}

// tblBorderTop: ┌────┬────┬────┐
func tblBorderTop(widths []int) string {
	s := "┌"
	for i, wd := range widths {
		if i > 0 {
			s += "┬"
		}
		s += strings.Repeat("─", wd+2)
	}
	s += "┐"
	return s
}

// tblBorderMid: ├────┼────┼────┤
func tblBorderMid(widths []int) string {
	s := "├"
	for i, wd := range widths {
		if i > 0 {
			s += "┼"
		}
		s += strings.Repeat("─", wd+2)
	}
	s += "┤"
	return s
}

// tblBorderBot: └────┴────┴────┘
func tblBorderBot(widths []int) string {
	s := "└"
	for i, wd := range widths {
		if i > 0 {
			s += "┴"
		}
		s += strings.Repeat("─", wd+2)
	}
	s += "┘"
	return s
}

// tblCell: │ val  │ val  │ val  │
func tblCell(widths []int, get func(int) string) string {
	s := "│"
	for i, wd := range widths {
		v := get(i)
		pad := wd - displayWidth(v)
		if pad < 0 {
			pad = 0
		}
		s += " " + v + strings.Repeat(" ", pad) + " │"
	}
	return s
}

func formatTLSCell(expiresAt time.Time, err error) string {
	status := tlsStatus(expiresAt, err)
	switch status {
	case "ERROR":
		return "ERROR"
	case "EXPIRED":
		return "EXPIRED"
	case "UNKNOWN":
		return "—"
	case "WARNING":
		return fmt.Sprintf("WARNING (%d д)", daysUntilExpiration(expiresAt))
	default: // OK
		return fmt.Sprintf("OK (%d д)", daysUntilExpiration(expiresAt))
	}
}

func tlsColorEx(cell string) string {
	switch {
	case strings.HasPrefix(cell, "OK"):
		return colorGreen
	case strings.HasPrefix(cell, "WARNING"):
		return colorYellow
	case strings.HasPrefix(cell, "EXPIRED") || strings.HasPrefix(cell, "ERROR"):
		return colorRed
	default:
		return ""
	}
}

func extractState(status string) string {
	if strings.HasSuffix(status, " UP") {
		return "UP"
	}
	if strings.HasSuffix(status, " DOWN") {
		return "DOWN"
	}
	return status
}

// ─── ИТОГО ────────────────────────────────────────────────────────────────────

func printSummary(w io.Writer, report Report) error {
	var lines []string

	up := itoa(report.UpCount)
	down := itoa(report.DownCount)
	if report.UpCount > 0 {
		up = colorize(colorGreen, itoa(report.UpCount))
	}
	if report.DownCount > 0 {
		down = colorize(colorRed, itoa(report.DownCount))
	}

	lines = append(lines,
		fmt.Sprintf("Проверено:  %d     Доступно:  %s     Недоступно:  %s", report.Total, up, down),
		fmt.Sprintf("Время:      %.2f s", report.CheckDuration.Seconds()),
	)

	if len(report.Reasons) > 0 {
		reasons := make([]string, 0, len(report.Reasons))
		for reason := range report.Reasons {
			reasons = append(reasons, reason)
		}
		sort.Strings(reasons)

		var sb strings.Builder
		for i, reason := range reasons {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s: %d", reason, report.Reasons[reason])
		}
		lines = append(lines, "Причины:   "+colorize(colorRed, sb.String()))
	}

	return printBoxSection(w, "ИТОГО", lines)
}

// ─── printReport (entry point) ────────────────────────────────────────────────

func printReport(w io.Writer, report Report) error {
	var err error
	sep := func() {
		if err == nil {
			_, err = fmt.Fprintln(w)
		}
	}

	if err = printNetworkInfo(w, report.Network); err != nil {
		return err
	}
	sep()
	if err = printSystemInfo(w, report.System); err != nil {
		return err
	}
	sep()
	if err = printResultsTable(w, report.Results); err != nil {
		return err
	}
	sep()
	if err = printSummary(w, report); err != nil {
		return err
	}
	return err
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
