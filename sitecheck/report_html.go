// Package main — sitecheck: однократная проверка доступности списка сайтов.
package main

import (
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// HTML-экспорт: self-contained страница (встроенный CSS, без внешних
// зависимостей) с карточками-сводкой, таблицей результатов и секциями
// СЕТЬ / СИСТЕМА / Причины.
func writeHTML(w io.Writer, report Report, generatedAt time.Time) error {
	var sb strings.Builder

	sb.WriteString(htmlHead())

	// CSS в начало body: так стили работают и в полном документе (stdout/файл),
	// и в body-only фрагменте (напр., публикация как самодостаточный HTML).
	sb.WriteString(`<style>`)
	sb.WriteString(htmlCSS)
	sb.WriteString(`</style>`)

	// ── Header ─────────────────────────────────────────────────────────────
	sb.WriteString(`<div class="page">`)
	sb.WriteString(`<header class="head">`)
	sb.WriteString(`<div class="head-left">`)
	sb.WriteString(`<h1>Отчёт <span>sitecheck</span></h1>`)
	fmt.Fprintf(&sb, `<p class="meta">Сформирован %s</p>`,
		html.EscapeString(generatedAt.Format(time.RFC1123)))
	sb.WriteString(`</div>`)
	sb.WriteString(`<a class="head-link" href="https://github.com/Def404/sitecheck" target="_blank" rel="noopener">` +
		`<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>` +
		`GitHub</a>`)
	sb.WriteString(`</header>`)

	// ── Summary cards ──────────────────────────────────────────────────────
	sb.WriteString(`<section class="cards">`)
	upClass := "ok"
	downClass := "bad"
	if report.UpCount == 0 {
		upClass = "muted"
	}
	if report.DownCount == 0 {
		downClass = "muted"
	}
	writeCard(&sb, "total", "Проверено", fmt.Sprint(report.Total), "muted")
	writeCard(&sb, "up", "Доступно", fmt.Sprint(report.UpCount), upClass)
	writeCard(&sb, "down", "Недоступно", fmt.Sprint(report.DownCount), downClass)
	writeCard(&sb, "time", "Время", fmt.Sprintf("%.2f s", report.CheckDuration.Seconds()), "muted")
	sb.WriteString(`</section>`)

	// ── Results table ──────────────────────────────────────────────────────
	sb.WriteString(`<section class="panel">`)
	sb.WriteString(`<h2>Результаты</h2>`)
	writeResultsTableHTML(&sb, report.Results)
	sb.WriteString(`</section>`)

	// ── Network & System ───────────────────────────────────────────────────
	sb.WriteString(`<section class="grid2">`)
	writeInfoCard(&sb, "Сеть", networkInfoRows(report.Network))
	writeInfoCard(&sb, "Система", systemInfoRows(report.System))
	sb.WriteString(`</section>`)

	// ── Reasons ────────────────────────────────────────────────────────────
	if len(report.Reasons) > 0 {
		sb.WriteString(`<section class="panel">`)
		sb.WriteString(`<h2>Причины недоступности</h2>`)
		writeReasonsHTML(&sb, report.Reasons)
		sb.WriteString(`</section>`)
	}

	sb.WriteString(`</div></body></html>`)

	_, err := io.WriteString(w, sb.String())
	return err
}

// writeHTMLToBoth печатает HTML в stdout и дублирует в файл path.
func writeHTMLToBoth(report Report, generatedAt time.Time, path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create HTML file %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close HTML file %s: %w", path, cerr))
		}
	}()

	if err = writeHTML(io.MultiWriter(os.Stdout, f), report, generatedAt); err != nil {
		return fmt.Errorf("write HTML to %s: %w", path, err)
	}
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func writeCard(sb *strings.Builder, id, label, value, tone string) {
	fmt.Fprintf(sb,
		`<div class="card %s"><div class="card-value" id="%s">%s</div><div class="card-label">%s</div></div>`,
		tone, html.EscapeString(id), html.EscapeString(value), html.EscapeString(label))
}

func writeResultsTableHTML(sb *strings.Builder, results []Result) {
	sb.WriteString(`<table>`)
	sb.WriteString(`<thead><tr>`)
	sb.WriteString(`<th>URL</th><th>Статус</th><th>State</th><th>Время</th>`)
	sb.WriteString(`<th>Попытки</th><th>IP</th><th>PING</th><th>TLS</th><th>Причина</th>`)
	sb.WriteString(`</tr></thead>`)
	sb.WriteString(`<tbody>`)

	for _, r := range results {
		sb.WriteString(`<tr>`)

		host := r.URL
		if u, err := parseURL(r.URL); err == nil {
			host = u.Host
		}
		fmt.Fprintf(sb, `<td class="url">%s</td>`, html.EscapeString(host))

		// Статус
		statusCell := "—"
		if r.StatusCode > 0 {
			statusCell = fmt.Sprintf("%d", r.StatusCode)
		}
		fmt.Fprintf(sb, `<td>%s</td>`, html.EscapeString(statusCell))

		// State badge
		state := "—"
		cls := ""
		switch {
		case r.Up:
			state, cls = "UP", "up"
		case !r.Skipped:
			state, cls = "DOWN", "down"
		}
		fmt.Fprintf(sb, `<td><span class="badge %s">%s</span></td>`, cls, state)

		// Время
		if r.Skipped {
			sb.WriteString(`<td>—</td>`)
		} else {
			fmt.Fprintf(sb, `<td>%d ms</td>`, r.Duration.Milliseconds())
		}

		// Попытки
		if r.Skipped {
			sb.WriteString(`<td>—</td>`)
		} else {
			fmt.Fprintf(sb, `<td>%d</td>`, r.Attempts)
		}

		// IP
		ip := "—"
		if len(r.IPs) > 0 {
			ip = r.IPs[0]
		}
		fmt.Fprintf(sb, `<td class="mono">%s</td>`, html.EscapeString(ip))

		// PING
		fmt.Fprintf(sb, `<td>%s</td>`, html.EscapeString(formatPingCell(r.Ping)))

		// TLS
		tlsCell := "—"
		if u, err := parseURL(r.URL); err == nil && u.Scheme == "https" {
			tlsCell = formatTLSCell(r.TLSExpiresAt, r.TLSErr)
		}
		fmt.Fprintf(sb, `<td>%s</td>`, html.EscapeString(tlsCell))

		// Причина
		reason := r.Reason
		if r.Skipped {
			reason = "не проверено"
		}
		fmt.Fprintf(sb, `<td class="reason">%s</td>`, html.EscapeString(reason))

		sb.WriteString(`</tr>`)
	}

	sb.WriteString(`</tbody></table>`)
}

func systemInfoRows(s SystemInfo) [][2]string {
	return [][2]string{
		{"ОС", s.OS},
		{"Пользователь", s.Username},
		{"Хост", s.Hostname},
	}
}

func networkInfoRows(n NetworkInfo) [][2]string {
	var rows [][2]string
	add := func(label, value string) { rows = append(rows, [2]string{label, value}) }

	if n.LocalIP != "" {
		add("Локальный IP", n.LocalIP)
	}
	if n.PrimaryIPError != "" {
		add("Исходящий IP", "недоступен")
	} else if n.PrimaryIP != "" {
		add("Исходящий IP", n.PrimaryIP)
	}

	switch {
	case n.InterfacesError != "":
		add("Шлюз", "невозможно определить")
	case n.LANError != "":
		add("Шлюз", "недоступен")
	case n.Gateway.IP != "":
		add("Шлюз", n.Gateway.IP+" ("+n.Gateway.Interface+")")
		if n.GatewayPingError != "" {
			add("Ping шлюза", "ошибка")
		} else if n.GatewayPing.Ran {
			add("Ping шлюза", formatPingCell(n.GatewayPing))
		}
	}

	switch {
	case n.DNSError != "":
		add("DNS-серверы", "недоступны")
	case len(n.DNSInterfaces) > 0:
		for _, di := range n.DNSInterfaces {
			add("DNS ("+di.Interface+")", strings.Join(di.Servers, ", "))
		}
	case len(n.DNSServers) > 0:
		add("DNS-серверы", strings.Join(n.DNSServers, ", "))
	}

	switch {
	case n.InterfacesError != "":
		add("VPN", "невозможно определить")
	case len(n.VPNInterfaces) > 0:
		parts := make([]string, 0, len(n.VPNInterfaces))
		for _, v := range n.VPNInterfaces {
			parts = append(parts, v.Name+" — "+v.IP)
		}
		add("VPN", strings.Join(parts, ", "))
		if n.VPNClient.Detected {
			add("VPN клиент", n.VPNClient.Type)
		} else {
			add("VPN клиент", "не определён")
		}
	default:
		add("VPN", "не активен")
	}

	if n.LocalIPsError != "" {
		add("Все IPv4", "недоступны")
	} else if len(n.LocalIPs) > 0 {
		add("Все IPv4", strings.Join(n.LocalIPs, ", "))
	}

	return rows
}

func writeInfoCard(sb *strings.Builder, title string, rows [][2]string) {
	fmt.Fprintf(sb, `<div class="panel"><h2>%s</h2><dl class="info">`, html.EscapeString(title))
	for _, row := range rows {
		fmt.Fprintf(sb, `<div class="info-row"><dt>%s</dt><dd>%s</dd></div>`,
			html.EscapeString(row[0]), html.EscapeString(row[1]))
	}
	sb.WriteString(`</dl></div>`)
}

func writeReasonsHTML(sb *strings.Builder, reasons map[string]int) {
	items := make([]string, 0, len(reasons))
	for r := range reasons {
		items = append(items, r)
	}
	sort.Strings(items)

	sb.WriteString(`<ul class="reasons">`)
	for _, r := range items {
		fmt.Fprintf(sb, `<li><span class="reason-name">%s</span><span class="reason-count">%d</span></li>`,
			html.EscapeString(r), reasons[r])
	}
	sb.WriteString(`</ul>`)
}

// htmlHead — <html><head> без стилей (CSS добавляется отдельно — htmlCSS).
// Подключает Inter из Google Fonts; при отсутствии сети шрифт откатится на
// системный стек из font-family (см. htmlCSS).
func htmlHead() string {
	return `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>sitecheck — отчёт</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
</head>
<body>`
}

// htmlCSS — встроенные стили. Вставляются в начало body (тег <style> допустим
// в body), поэтому стили работают и в полном документе (stdout/файл), и в
// body-only фрагменте (напр., публикация как самодостаточный HTML).
const htmlCSS = `:root{
  --bg:#0b1120; --panel:#111a2e; --panel-2:#0e1626;
  --border:#1f2b45; --text:#f1f5fb; --muted:#a4b3cc;
  --accent:#38bdf8;
  --ok:#22c55e; --bad:#ef4444; --warn:#eab308; --muted-c:#64748b;
}
*{box-sizing:border-box}
body{
  margin:0; padding:32px 16px;
  font-family:"Inter",system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
  font-size:15px;
  background:var(--bg); color:var(--text); line-height:1.6;
  -webkit-font-smoothing:antialiased; text-rendering:optimizeLegibility;
}
.page{max-width:1080px;margin:0 auto}
.head{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;margin-bottom:24px}
.head-left{min-width:0}
.head-link{flex:0 0 auto;display:inline-flex;align-items:center;gap:7px;color:var(--muted);text-decoration:none;font-size:13px;font-weight:500;padding:4px 12px;border:1px solid var(--border);border-radius:8px;transition:color .15s,border-color .15s}
.head-link svg{flex:0 0 auto}
.head-link:hover{color:var(--text);border-color:var(--muted)}
h1{font-size:28px;font-weight:700;margin:0 0 4px;letter-spacing:.2px}
h1 span{color:var(--text)}
.meta{color:var(--muted);font-size:14px;margin:0}
h2{font-size:14px;font-weight:600;text-transform:uppercase;letter-spacing:.6px;color:var(--muted);margin:0 0 14px}
.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:20px}
.card{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:16px 18px}
.card-value{font-size:30px;font-weight:700}
.card-label{font-size:13px;color:var(--muted);margin-top:2px}
.card.ok .card-value{color:var(--ok)}
.card.bad .card-value{color:var(--bad)}
.card.muted .card-value{color:var(--text)}
.panel{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:20px;margin-bottom:20px}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:20px}
table{width:100%;border-collapse:collapse;font-size:15px}
th,td{text-align:left;padding:11px 12px;border-bottom:1px solid var(--border)}
th{font-size:12px;text-transform:uppercase;letter-spacing:.5px;color:var(--muted);font-weight:600}
tbody tr:hover{background:var(--panel-2)}
tbody tr:last-child td{border-bottom:none}
td.url{font-weight:600;max-width:340px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
td.mono,td.url{font-variant-numeric:tabular-nums}
.badge{display:inline-block;padding:2px 10px;border-radius:999px;font-size:12px;font-weight:700}
.badge.up{background:rgba(34,197,94,.15);color:var(--ok)}
.badge.down{background:rgba(239,68,68,.15);color:var(--bad)}
.reason{color:var(--bad)}
.info-row{display:flex;justify-content:space-between;gap:16px;padding:7px 0;border-bottom:1px solid var(--border)}
.info-row:last-child{border-bottom:none}
.info-row dt{color:var(--muted);flex:0 0 auto}
.info-row dd{margin:0;font-weight:500;text-align:right;word-break:break-word}
.reasons{list-style:none;margin:0;padding:0}
.reasons li{display:flex;justify-content:space-between;padding:8px 0;border-bottom:1px solid var(--border)}
.reasons li:last-child{border-bottom:none}
.reason-name{color:var(--text)}
.reason-count{font-variant-numeric:tabular-nums;color:var(--bad);font-weight:700}
@media (max-width:640px){
  .cards{grid-template-columns:repeat(2,1fr)}
  .grid2{grid-template-columns:1fr}
}`
