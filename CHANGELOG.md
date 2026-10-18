# Changelog

Все значимые изменения в этом проекте документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
версионирование — по [Semantic Versioning](https://semver.org/lang/ru/).

## [0.3.0] — 2026-10-19

### Добавлено
- Таблица результатов вместо построчного вывода (TODOS #36): URL | Статус |
  Время | TLS | Причина. Ящики `┌─┐│├─┤└─┘` для секций СЕТЬ, СИСТЕМА, ИТОГО.
  Русские заголовки, динамическая ширина колонок.

## [0.2.0] — 2026-10-19

### Добавлено
- ANSI-цвета в консольном выводе (TODOS #35): UP — зелёный, DOWN — красный,
  TLS WARNING — жёлтый, заголовки секций — bold. Цвета автоматически
  отключаются, если stdout не TTY (`golang.org/x/term`).

### Исправлено
- 22 находки golangci-lint: errcheck (5), gosec G306 (4),
  revive (8), staticcheck S1008/QF1003 (3), unparam (1), unused (2).
- `.golangci.yml` приведён к валидной схеме v2.15.0
  (`linters-settings` → `linters.settings`, `issues.exclude-dirs` → `run.exclude-dirs`).

## [0.1.0] — 2026-10-19

### Добавлено
- Go CLI-инструмент `sitecheck` — однократная проверка доступности списка URL
  через HTTP GET, с DNS-резолвингом и определением срока действия TLS-сертификата.
- Флаги: `--concurrency`, `--retries`, `--json`, `--json-report`, `--output`,
  `--text-output`, `--version`.
- Блок сведений о системе в отчёте: hostname, username, OS
  (Linux / macOS / Windows).
- Блок сведений о сети в отчёте: локальный IP, исходящий IP, шлюз (с ping),
  DNS-серверы (resolved / resolv.conf / macOS / Windows), VPN-интерфейсы,
  все локальные IPv4.
- Кроссплатформенный диспетчер ping шлюза (`pingGateway`) с реализациями
  для каждой ОС.
- Цели кросс-компиляции: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
  windows/amd64.
- `Makefile` с целями: `build`, `test`, `vet`, `lint`, `fmt`, `fmt-check`,
  `tidy`, `clean`, `cross`, `help`.
- GitHub Actions CI: check (fmt, vet, golangci-lint, test -race, build),
  кросс-компиляция (матрица 5 платформ), release по тегам `v*`.
- `.golangci.yml` (v2) с linters: govet, errcheck, staticcheck, unused,
  gosimple, gofmt, goimports, gosec, revive, misspell, unconvert, unparam,
  prealloc, ineffassign.
- Классификация retryable-ошибок: `isRetryableError()` в `errors.go` —
  DeadlineExceeded → повтор, TLS → без повтора, DNS → `IsTemporary`,
  connection refused → повтор, generic → повтор.
- Задержка 200 мс между повторными попытками (фиксированная, см. `app.go`).
- `--version` выводит `sitecheck <версия> (commit <hash>, <дата>, <os>/<arch>)`.
- `TODOS.md` — приоритизированный список задач.

### Изменено
- Python-реализация заменена на Go.
- `go.mod` перенесён в корень репозитория; исходные файлы в `sitecheck/`.
- Код разбит из единого `main.go` (864 строки) на 14 модульных файлов,
  все `package main`.
- CI: `golangci-lint-action` заменяет `make lint` в GitHub Actions;
  версия линтера зафиксирована с префиксом `v`.
- Конфигурация `golangci-lint` мигрирована на схему v2.

### Удалено
- `checker-go/` (архивный Go-прототип).
- `checker.py` (исходная Python-реализация).

### Исправлено
- DNS / TLS не выполняются, если HTTP-запрос уже завершился ошибкой
  (отмена контекста, невалидный URL).
- `isConnectionRefused` — работает только на Linux через
  `syscall.ECONNREFUSED` (см. `TODOS.md` п. 3 для macOS/Windows).
