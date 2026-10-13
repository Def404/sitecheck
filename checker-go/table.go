package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// ANSI colors
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	cyan    = "\033[36m"
)

var ansiRe = regexp.MustCompile(`\033\[[0-9;]*m`)

func isTTY() bool {
	fi, _ := os.Stdout.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func c(text, color string) string {
	if !isTTY() {
		return text
	}
	return color + text + reset
}

func visualLen(s string) int {
	return utf8.RuneCountInString(ansiRe.ReplaceAllString(s, ""))
}

func padRight(s string, width int) string {
	cur := visualLen(s)
	if cur >= width {
		return s
	}
	return s + strings.Repeat(" ", width-cur)
}

func col(s string, width int) string {
	return padRight(s, width)
}

func formatResult(r CheckResult, pingEnabled bool) string {
	var flag string
	if r.OK {
		flag = c("✔ UP", green)
	} else {
		flag = c("✘ DOWN", red)
	}
	flag = col(flag, 9)

	status := r.HTTPStatus
	if len(status) > 14 {
		status = status[:13] + "…"
	}
	httpCol := col(fmt.Sprintf("%10s  %6.2fs", status, r.HTTPElapsed), 19)

	host := r.Host
	if r.IP != "" {
		host += c(" ("+r.IP+")", dim)
	}

	parts := []string{flag, httpCol}
	if pingEnabled && r.Ping != nil {
		parts = append(parts, col(r.Ping.StatusStr(), 26))
	}
	parts = append(parts, host)

	return "  " + strings.Join(parts, "  ")
}

func renderRound(results []CheckResult, prev map[string]bool, roundNo int, pingEnabled bool) string {
	ts := time.Now().Format("15:04:05")
	pingTag := ""
	if pingEnabled {
		pingTag = " + ICMP"
	}

	var lines []string
	for _, r := range results {
		lines = append(lines, formatResult(r, pingEnabled))
	}

	var header string
	if pingEnabled {
		header = col("Статус", 9) + "  " + col("HTTP", 19) + "  " + col("Пинг", 26) + "  " + "Хост"
	} else {
		header = col("Статус", 9) + "  " + col("HTTP", 19) + "  " + "Хост"
	}

	width := visualLen(header)
	for _, l := range lines {
		if visualLen(l) > width {
			width = visualLen(l)
		}
	}
	if width < 40 {
		width = 40
	}
	sep := strings.Repeat("─", width+2)

	var out []string
	out = append(out, c("┌"+sep+"┐", dim))
	title := col(fmt.Sprintf(" Проверка #%d  ·  %d сайт(ов)%s  ·  %s ", roundNo, len(results), pingTag, ts), width)
	out = append(out, c("│", dim)+title+c("│", dim))
	out = append(out, c("├"+sep+"┤", dim))
	out = append(out, c("│ ", dim)+header+col("", width-visualLen(header))+c(" │", dim))
	out = append(out, c("├"+sep+"┤", dim))

	for _, line := range lines {
		out = append(out, c("│ ", dim)+col(line, width)+c(" │", dim))
	}

	out = append(out, c("├"+sep+"┤", dim))

	ups, downs := 0, 0
	for _, r := range results {
		if r.OK {
			ups++
		} else {
			downs++
		}
	}
	upStr := c(fmt.Sprintf("%d доступно", ups), green)
	downStr := c(fmt.Sprintf("%d недоступно", downs), dim)
	if downs > 0 {
		downStr = c(fmt.Sprintf("%d недоступно", downs), red)
	}
	summary := col(fmt.Sprintf(" Итого: %s, %s", upStr, downStr), width)
	out = append(out, c("│", dim)+summary+c("│", dim))
	out = append(out, c("└"+sep+"┘", dim))

	// Changes
	var changes []string
	for _, r := range results {
		if old, exists := prev[r.URL]; exists && old != r.OK {
			dir := "up → down"
			if r.OK {
				dir = "down → up"
			}
			color := red
			if r.OK {
				color = green
			}
			changes = append(changes, fmt.Sprintf("  %s%s%s %s%s: %s", c("●", color), "", reset, c(r.Host, bold), reset, dir))
		}
	}
	if len(changes) > 0 {
		out = append(out, "")
		out = append(out, c("  Изменения:", yellow))
		out = append(out, changes...)
	} else if roundNo > 1 {
		out = append(out, c("  Изменения: нет", dim))
	}

	// DOWN reasons
	var downSites []CheckResult
	for _, r := range results {
		if !r.OK {
			downSites = append(downSites, r)
		}
	}
	if len(downSites) > 0 {
		out = append(out, "")
		out = append(out, c("  Причины недоступности:", yellow))
		for _, r := range downSites {
			reason := r.HTTPStatus
			if srv, ok := r.Headers["Server"]; ok {
				reason += "  (" + srv + ")"
			}
			if r.ErrBody != "" {
				firstLine := r.ErrBody
				if idx := strings.Index(firstLine, "\n"); idx > 0 {
					firstLine = firstLine[:idx]
				}
				if len(firstLine) > 80 {
					firstLine = firstLine[:80]
				}
				reason += "  |  " + firstLine
			}
			out = append(out, fmt.Sprintf("    %s%s%s %s%s: %s%s%s", c("✘", red), "", reset, r.Host, reset, c(reason, dim), "", reset))
		}
	}

	return strings.Join(out, "\n")
}

var lastLineCount = 0

func dynamicPrint(table string, interactive bool) {
	if !interactive {
		fmt.Println(table)
		return
	}
	lines := strings.Split(table, "\n")
	// Move cursor up and clear
	for i := 0; i < lastLineCount; i++ {
		fmt.Print("\033[1A\033[2K")
	}
	fmt.Println(table)
	lastLineCount = len(lines)
}

func clearScreen() {
	for i := 0; i < lastLineCount; i++ {
		fmt.Print("\033[1A\033[2K")
	}
	lastLineCount = 0
}
