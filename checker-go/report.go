package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func generateReport(
	results []CheckResult,
	roundNo int,
	sysinfo SystemInfo,
	vpnName string,
	vpnIPs []string,
	prev map[string]bool,
	sessionStart time.Time,
	pingEnabled bool,
) string {
	ts := time.Now().Format("2006-01-02 15:04:05")
	uptime := time.Since(sessionStart).Seconds()
	uptimeStr := fmt.Sprintf("%.0fs", uptime)
	if uptime > 120 {
		uptimeStr = fmt.Sprintf("%.1fmin", uptime/60)
	}

	W := 70
	var b strings.Builder

	b.WriteString(strings.Repeat("=", W) + "\n")
	b.WriteString("  Отчёт мониторинга сайтов\n")
	b.WriteString(fmt.Sprintf("  Проверка #%d  ·  %s  ·  время сессии: %s\n", roundNo, ts, uptimeStr))
	b.WriteString(strings.Repeat("=", W) + "\n")

	// Network
	gw := getGatewayIP()
	b.WriteString("\n[СЕТЬ]\n")
	b.WriteString(fmt.Sprintf("  Локальный IP:   %s\n", sysinfo.IP))
	b.WriteString(fmt.Sprintf("  Шлюз:           %s  (%s)\n", gw, pingGateway(gw)))
	b.WriteString(fmt.Sprintf("  Маршрут:        %s\n", getDefaultRoute()))
	b.WriteString(fmt.Sprintf("  DNS-резолверы:  %s\n", getDNSResolvers()))
	b.WriteString(fmt.Sprintf("  VPN:            %s\n", formatVPNStatus(vpnName, vpnIPs)))

	// System
	b.WriteString("\n[СИСТЕМА]\n")
	b.WriteString(fmt.Sprintf("  OS:           %s\n", sysinfo.OS))
	b.WriteString(fmt.Sprintf("  Пользователь: %s@%s\n", sysinfo.User, sysinfo.Host))

	// Results
	ups, downs := 0, 0
	for _, r := range results {
		if r.OK {
			ups++
		} else {
			downs++
		}
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("[РЕЗУЛЬТАТЫ]  %d UP  /  %d DOWN  из %d\n", ups, downs, len(results)))
	b.WriteString(strings.Repeat("-", W) + "\n")

	for _, r := range results {
		status := "UP "
		if !r.OK {
			status = "DOWN"
		}
		ipStr := ""
		if r.IP != "" {
			ipStr = fmt.Sprintf("  ->  %s", r.IP)
		}
		b.WriteString(fmt.Sprintf("  [%s]  %10s  %6.2fs  %s%s\n", status, r.HTTPStatus, r.HTTPElapsed, r.URL, ipStr))

		// DNS
		dnsHost := hostWithoutPort(r.Host)
		dnsResult := doNslookup(dnsHost)
		b.WriteString(fmt.Sprintf("           DNS: %s\n", dnsResult))

		// DOWN details
		if !r.OK {
			reason := r.HTTPStatus
			if srv, ok := r.Headers["Server"]; ok {
				reason += "  (server: " + srv + ")"
			}
			if r.ErrBody != "" {
				firstLine := r.ErrBody
				if idx := strings.Index(firstLine, "\n"); idx > 0 {
					firstLine = firstLine[:idx]
				}
				if len(firstLine) > 120 {
					firstLine = firstLine[:120]
				}
				reason += "  |  " + firstLine
			}
			b.WriteString(fmt.Sprintf("           причина: %s\n", reason))
		}

		// TLS
		if r.OK {
			if tlsName, ok := r.Headers["TLS"]; ok {
				b.WriteString(fmt.Sprintf("           TLS: %s\n", tlsName))
			}
		}
	}

	// Ping
	if pingEnabled {
		b.WriteString("\n[ICMP-ПИНГ]\n")
		for _, r := range results {
			if r.Ping != nil {
				b.WriteString(fmt.Sprintf("  %-40s  %s\n", r.Host, r.Ping.StatusStr()))
			}
		}
	}

	// Changes
	var changes []string
	for _, r := range results {
		if old, exists := prev[r.URL]; exists && old != r.OK {
			dir := "up -> down"
			if r.OK {
				dir = "down -> up"
			}
			changes = append(changes, fmt.Sprintf("  %s: %s", r.Host, dir))
		}
	}
	if len(changes) > 0 {
		b.WriteString("\n[ИЗМЕНЕНИЯ]\n")
		b.WriteString(strings.Join(changes, "\n") + "\n")
	}

	b.WriteString("\n" + strings.Repeat("=", W) + "\n\n")
	return b.String()
}

func appendToFile(path, text string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка записи в %s: %v\n", path, err)
		return
	}
	defer f.Close()
	f.WriteString(text)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
