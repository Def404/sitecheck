# Changelog

Все значимые изменения в этом проекте документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
версионирование — по [Semantic Versioning](https://semver.org/lang/ru/).

## [0.5.0] — 2026-10-25

### Добавлено
- Определение типа VPN-клиента по запущенным процессам (Linux, macOS, Windows):
  OpenVPN, WireGuard, Amnezia. В отчёте выводится строка `VPN клиент: <тип>`.

### Изменено
- `make lint` теперь находит golangci-lint в `$GOPATH/bin`, если он не в `PATH`.

## [0.4.3] — 2026-10-25

### Исправлено
- 4 находки `errcheck` в `progress_test.go`: `p.finish()` в тестовых
  функциях вызывался без проверки возвращаемой ошибки; теперь каждая
  запись `if err := p.finish(); err != nil { t.Errorf(...) }`.

## [0.4.2] — 2026-10-25

### Исправлено
- 4 находки golangci-lint, сломанные в 0.4.0: `errcheck` —
  `resp.Body.Close` в `main_test.go` (тест закрытия тела через `t.Cleanup`
  с проверкой ошибки), `p.w.Write` в `tick()` и `finish()` прогресс-бара
  (ошибка записи запоминается и возвращается из `finish()`); `revive`
  `unused-parameter` — аргумент `newHTTPClient` переименован в `_`
  (тип `Options` сохранён как задел под TLS/Transport, TODOS #39).

## [0.4.1] — 2026-10-25

### Исправлено
- gofmt: выравнивание полей в `main_test.go` и комментария в `progress_test.go`,
  сломанное в 0.4.0; CI-шаг `gofmt check` падает на 0.4.0.

## [0.4.0] — 2026-10-20

### Добавлено
- Флаг `--help` (TODOS #24): печатает строку использования и блок примеров
  на stdout (exit 0), обрабатывается до валидации аргументов.
- Прерывание по Ctrl+C (TODOS #31): `run()` использует `signal.NotifyContext`
  (SIGINT/SIGTERM). При прерывании все активные HTTP-запросы, DNS и
  TLS-проверки немедленно отменяются, незапущенные проверки пропускаются,
  выводится частичный отчёт, процесс завершается с кодом **130**.
- Graceful shutdown (TODOS #32): после сигнала ожидаются текущие проверки
  (до 5 s), затем печатается частичный отчёт с пометкой «не проверено»
  для непроверенных URL.
- Прогресс-бар (TODOS #37): при нескольких URL строка
  `[██████░░░░] 60% (3/5)` обновляется через `\r` на TTY и очищается
  перед отчётом. Не выводится при `--json`, `--json-report`,
  `--text-output` и при одном URL.

### Изменено
- Конфигурация `http.Client` вынесена в `newHTTPClient(opts)` (TODOS #30) —
  центральное место для настроек клиента (redirect, future: TLS, Transport).

### Исправлено
- Прогресс-бар больше не впечатывается в отчёт. Обычный запуск ждёт все
  проверки; лимит 5 с действует только после Ctrl+C или SIGTERM. Обновления
  полосы после её очистки не рисуются.

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

[Unreleased]: https://github.com/Def404/sitecheck/compare/v0.4.3...HEAD
[0.4.3]: https://github.com/Def404/sitecheck/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/Def404/sitecheck/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/Def404/sitecheck/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/Def404/sitecheck/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Def404/sitecheck/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Def404/sitecheck/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Def404/sitecheck/releases/tag/v0.1.0
