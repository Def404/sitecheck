# sitecheck

Go CLI-инструмент для однократной проверки доступности списка сайтов: HTTP GET, DNS-резолвинг, TLS-сертификат, сведения о системе и сети.

## Структура

- `go.mod` / `go.sum` — в корне репозитория (модуль `sitecheck`)
- `sitecheck/` — все исходные файлы, `package main`
- `Makefile` — сборка, тесты, lint, кросс-компиляция
- `.github/workflows/ci.yml` — CI (check, cross-compile, release по тегам `v*`)
- `TODOS.md` — приоритизированный список задач
- `CHANGELOG.md` — changelog на русском (Keep a Changelog / SemVer)
- `LICENSE` — MIT

## Сборка и проверки

```bash
make build       # → dist/sitecheck
make test        # go test -race -count=1 ./sitecheck/...
make vet         # go vet
make lint        # golangci-lint v2.15.0
make cross       # 5 платформ → dist/
```

## Ключевые файлы (sitecheck/)

| Файл | Назначение |
|---|---|
| `main.go` | `main()`, `printVersion()`, `ErrSitesDown`, ldflags-переменные |
| `app.go` | `run()`, `check()` — оркестрация |
| `config.go` | `Options`, `parseArgs()`, `getTimeout()` |
| `checker.go` | `Result`, `State()`, `parseURL()`, `isUp()` |
| `errors.go` | `isTimeout`, `isDNSError`, `isConnectionRefused`, `isTLSError`, `isRetryableError`, `statusReason`, `errorReason` |
| `input.go` | `loadURLs()` |
| `dns.go` | DNS-серверы (Linux/macOS/Windows), `parseResolvConf`, `parseResolvedDNS`, `InterfaceDNS` |
| `network.go` | `NetworkInfo`, `getNetworkInfo()`, gateway, VPN, LAN |
| `ping.go` | `pingGateway()` — кроссплатформенный ping |
| `tls.go` | `getTLSExpiration()`, `tlsStatus()`, `daysUntilExpiration()` |
| `system.go` | `SystemInfo`, `getSystemInfo()`, OS-имя (Linux/macOS/Windows) |
| `report.go` | `Report`, `buildReport()`, `countReasons()` |
| `report_text.go` | Текстовый отчёт: ящики, таблица, цвета |
| `report_json.go` | JSON-отчёт: `JSONReport`, `JSONResult`, marshal |
| `color.go` | ANSI-коды, `colorize()`, `stateColor()`, `displayWidth()` |
| `*_test.go` | Тесты |

## Конвенции

- **Ошибки:** `fmt.Errorf("контекст: %w", err)` — не возвращать голые ошибки
- **Контекст:** `context.Background()` создаётся только в `run()` (app.go), передаётся вниз; саб-функции принимают `ctx context.Context` первым параметром
- **Вывод:** функции принимают `io.Writer` первым параметром, используют `fmt.Fprintf(w, ...)`, возвращают `error`
- **Валидация флагов:** в `parseArgs()`, до сетевых операций
- **Структы-результаты:** создаются один раз вверху, поля заполняются инкрементально; non-fatal ошибки — в dedicated `*Error` поля
- **Перед проверкой derived-значений** (пустой срез) — проверять dedicated error field
- **Консольный вывод:** ящики `┌─┐│├─┤└─┘`, таблица `┌┬┐│├┼┤└┴┘`, русские заголовки, ANSI-цвета (off при не-TTY), самодостаточные строки
- **go.mod:** лицензия помечена комментарием `// License: MIT — see LICENSE.`

## Версионирование

- `Makefile`: `VERSION ?= 0.3.0`
- `ldflags`: `-X main.version` `-X main.commit` `-X main.buildDate`
- Тег: `v0.3.0` (annotated)
- CHANGELOG: на русском, секции «Добавлено / Изменено / Удалено / Исправлено»

## golangci-lint

- Версия: **v2.15.0** (в CI: `golangci-lint-action@v7`)
- Конфиг: `.golangci.yml` (схема v2: `linters.settings`, `run.exclude-dirs`)
- Linters: gosec, revive, misspell, unconvert, unparam, prealloc, ineffassign + standard
