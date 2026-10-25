# sitecheck

Однократная проверка доступности списка сайтов: HTTP GET, DNS-резолвинг, ping каждого хоста, срок действия TLS-сертификата, сведения о системе и сети.

Написан на Go (модуль `sitecheck`, Go 1.24+). Код — в каталоге [`sitecheck/`](sitecheck/), `go.mod` — в корне репозитория.

## Возможности

- Проверка списка URL из файла: HTTP GET, статус-код, время отклика
- DNS-резолвинг каждого хоста (IP-адрес в отчёте)
- Ping каждого хоста (latency в отчёте) — информационно, не влияет на UP/DOWN
- Срок действия TLS-сертификата (`https` только) с предупреждением при < 7 дней
- Повторные попытки: `--retries N` с фиксированным бэкоффом 200 мс
- Параллельная проверка: `--concurrency N` (по умолчанию 5)
- Прогресс-бар: `[██████░░░░] 60% (3/5)` при нескольких URL на TTY.
  Строка очищается перед отчётом. Не выводится при JSON-форматах
  и при записи отчёта в файл
- Текстовый отчёт: ящики `┌─┐│├─┤└─┘`, таблица результатов,
  ANSI-цвета (автоотключение при pipe/файле)
- Формат вывода: `--output-format text|json|json-report|csv|html` (по умолчанию `text`);
  `json` — массив результатов, `json-report` — полный отчёт,
  `csv` — табличный экспорт для Excel/Google Sheets,
  `html` — самодостаточная HTML-страница с современным дизайном
- Запись отчёта в файл: `--output FILE` для любого формата (stdout по умолчанию)
- Сведения о системе: hostname, user, OS (Linux / macOS / Windows)
- Сведения о сети: локальный IP, исходящий IP, шлюз (с ping), DNS-серверы,
  VPN-интерфейсы, все IPv4
- Кросс-платформенность: Linux, macOS, Windows

## Требования

- Go **1.24+** (для сборки из исходников)
- `ping` в PATH (для ping шлюза) — опционально, без него поле `Ping шлюза`
  будет помечено как ошибка

## Сборка

```bash
make build        # → dist/sitecheck
make test         # go test -race
make lint         # golangci-lint (если установлен)
make cross        # все 5 платформ → dist/
```

## Запуск

```bash
# Текстовый отчёт (по умолчанию)
./dist/sitecheck sites.txt

# JSON-массив результатов
./dist/sitecheck --output-format json sites.txt

# JSON-отчёт (система + сеть + результаты + итоги)
./dist/sitecheck --output-format json-report sites.txt

# JSON-отчёт в файл
./dist/sitecheck --output-format json-report --output report.json sites.txt

# Текстовый отчёт в файл
./dist/sitecheck --output-format text --output summary.txt sites.txt

# CSV-экспорт (stdout)
./dist/sitecheck --output-format csv sites.txt

# CSV-экспорт в файл
./dist/sitecheck --output-format csv --output report.csv sites.txt

# HTML-отчёт в файл (самодостаточная страница)
./dist/sitecheck --output-format html --output report.html sites.txt

# Параллельная проверка (8 потоков), 1 повтор после сбоя
./dist/sitecheck --concurrency 8 --retries 1 sites.txt

# Другой таймаут (по умолчанию 5s)
SITECHECK_TIMEOUT=10s ./dist/sitecheck sites.txt

# Версия
./dist/sitecheck --version
```

## Формат списка

Один URL на строку, схема `http` или `https`. Пустые строки и строки,
начинающиеся с `#`, пропускаются.

```
# production
https://example.com
https://api.example.com
http://internal.local:8080/health
```

> **Примечание:** строка без схемы (например, `example.com`) будет отклонена
> как invalid URL. Добавьте `https://` явно (см. `TODOS.md` п. 6 — автодобавление
> схемы в планах).

## Флаги

| Флаг | Описание | По умолчанию |
|---|---|---|
| `file` (позиционный) | путь к файлу со списком URL | — |
| `--output-format FORMAT` | формат вывода: `text` (отчёт), `json` (массив результатов), `json-report` (полный отчёт), `csv` (таблица для Excel/Sheets), `html` (HTML-страница) | `text` |
| `--output FILE` | записать отчёт в файл вместо stdout (любой формат) | stdout |
| `--concurrency N` | число параллельных проверок (положительное целое) | `5` |
| `--retries N` | число повторных попыток после сбоя (≥ 0) | `0` |
| `--version` | версия сборки (ldflags) и выход | — |
| `--help` | строка использования и примеры и выход | — |

## Переменные окружения

| Переменная | Описание | По умолчанию |
|---|---|---|
| `SITECHECK_TIMEOUT` | таймаут одного HTTP-запроса (формат `time.Duration`: `5s`, `100ms`, `1m`) | `5s` |

## Коды выхода

| Код | Значение |
|---|---|
| `0` | все сайты доступны |
| `1` | хотя бы один сайт недоступен **или** аргументы неверны |
| `130` | прервано пользователем (Ctrl+C / SIGTERM) |

Сообщения об ошибке аргументов — в stderr; отчёт — в stdout.

При Ctrl+C (SIGINT) или SIGTERM все активные HTTP-запросы немедленно
отменяются, незапущенные проверки пропускаются. Процесс дожидается
завершения текущих проверок (до 5 s) и печатает частичный отчёт —
непроверенные URL помечены как `не проверено`.

## Примеры вывода

### Текстовый (`sites.txt` → stdout)

```
┌───────────────────────────────────────────────────────────────────┐
│ СЕТЬ                                                              │
├───────────────────────────────────────────────────────────────────┤
│ Локальный IP:  192.168.1.100                                      │
│ Исходящий IP:  172.16.0.1                                         │
│ Шлюз:          192.168.1.1 (eth0)                                 │
│ Ping шлюза:    OK (0.85 ms)                                       │
│ DNS (eth0):    192.168.1.1                                        │
│ VPN:           не активен                                         │
│ Все IPv4:      192.168.1.100, 172.16.0.1                          │
└───────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────┐
│ СИСТЕМА                             │
├─────────────────────────────────────┤
│ ОС:      Ubuntu 24.04 LTS           │
│ Пользов: alex@host                  │
└─────────────────────────────────────┘

┌─────────────────────┬──────────┬─────────┬───────────────────┬──────────────────┬─────────────┬───────────────┐
│ URL                 │ Статус   │ Время   │ IP                │ PING             │ TLS         │ Причина       │
├─────────────────────┼──────────┼─────────┼───────────────────┼──────────────────┼─────────────┼───────────────┤
│ example.com         │ 200 UP   │ 142 ms  │ 93.184.216.34     │ OK (18.32 ms)    │ OK (88 д)   │               │
│ api.example.com     │ DOWN     │ 5012 ms │ —                 │ —                │ ERROR       │ DNS           │
└─────────────────────┴──────────┴─────────┴───────────────────┴──────────────────┴─────────────┴───────────────┘

┌───────────────────────────────────────────────────────────────────┐
│ ИТОГО                                                             │
├───────────────────────────────────────────────────────────────────┤
│ Проверено:  2     Доступно:  1     Недоступно:  1                 │
│ Время:      5.15 s                                                │
│ Причины:   DNS: 1                                                │
└───────────────────────────────────────────────────────────────────┘
```

> Цвета (UP — зелёный, DOWN — красный, TLS WARNING — жёлтый, заголовки — bold)
> автоматически отключаются при перенаправлении в файл или pipe.

### JSON-отчёт (`--output-format json-report`)

```json
{
    "system": {
        "hostname": "host",
        "username": "alex",
        "os": "Ubuntu 24.04 LTS"
    },
    "network": {
        "local_ip": "192.168.1.100",
        "primary_ip": "172.16.0.1",
        "gateway": { "ip": "192.168.1.1", "interface": "eth0" },
        "gateway_ping": { "ran": true, "success": true, "latency_ms": 0.85 },
        "dns_servers": ["192.168.1.1"],
        "vpn_interfaces": [],
        "local_ips": ["192.168.1.100", "172.16.0.1"]
    },
    "total": 2,
    "up": 1,
    "down": 1,
    "check_duration_ms": 5153,
    "reasons": { "DNS": 1 },
    "results": [
        {
            "url": "https://example.com",
            "status_code": 200,
            "up": true,
            "duration_ms": 142,
            "ip": "93.184.216.34",
            "ping": { "ran": true, "success": true, "latency_ms": 18.32 },
            "reason": "",
            "attempts": 1
        },
        {
            "url": "https://api.example.com",
            "status_code": 0,
            "up": false,
            "duration_ms": 5012,
            "reason": "DNS",
            "attempts": 2
        }
    ]
}
```

### JSON-массив результатов (`--output-format json`)

```json
[
  {
    "url": "https://example.com",
    "status_code": 200,
    "up": true,
    "duration_ms": 142,
    "ip": "93.184.216.34",
    "ping": { "ran": true, "success": true, "latency_ms": 18.32 },
    "reason": "",
    "attempts": 1
  },
  {
    "url": "https://api.example.com",
    "status_code": 0,
    "up": false,
    "duration_ms": 5012,
    "reason": "DNS",
    "attempts": 2
  }
]
```

### CSV-экспорт (`--output-format csv`)

```csv
URL,Status,State,Duration_ms,Attempts,Reason,IP,Ping_ms,TLS_expires,TLS_status
https://example.com,200,UP,142,1,,93.184.216.34,18.32,2026-11-01T00:00:00Z,OK
https://api.example.com,,DOWN,5012,2,DNS,,,
```

Колонки: `URL`, `Status` (HTTP-статус), `State` (`UP`/`DOWN`/`—` для
пропущенных), `Duration_ms`, `Attempts`, `Reason`, `IP` (первый из DNS),
`Ping_ms`, `TLS_expires` (RFC 3339, UTC), `TLS_status` (`OK`/`WARNING`/
`EXPIRED`/`ERROR`). Пустые ячейки — поле не применялось (например, ping
недоступен или сайт `http`).

### HTML-отчёт (`--output-format html`)

Самодостаточная HTML-страница (встроенный CSS, без внешних зависимостей):
карточки-сводка (Проверено / Доступно / Недоступно / Время), таблица
результатов с цветными бейджами UP/DOWN, секции «Сеть» и «Система», а при
наличии причин — блок «Причины недоступности». Тёмная тема, адаптивная
сетка. Все пользовательские значения экранируются (`html.EscapeString`).

```bash
./dist/sitecheck --output-format html --output report.html sites.txt
```

Откройте `report.html` в браузере.

## Состояние сайта

- `UP` — HTTP-статус `2xx` или `3xx` (redirect не следует, `CheckRedirect`
  возвращает `http.ErrUseLastResponse`)
- `DOWN` — ошибка запроса, статус `4xx`/`5xx`
- `PING: OK (X ms)` — хост отвечает на ICMP-echo (информационно, не влияет
  на UP/DOWN)
- `PING: FAIL` — хост не ответил на ICMP (информационно)
- `PING: —` — ping недоступен (нет утилиты, не поддерживается ОС)
- `TLS: WARNING` — сертификат истекает в ближайшие 7 дней
- `TLS: EXPIRED` — сертификат истёк (детектируется только при `http.Get`, если
  сервер принимает соединение; отдельный TLS-раздел в отчёте всегда показывает
  `NotAfter`)

## Кроссплатформенность

| Платформа | DNS | Шлюз | Ping шлюза | Ping хоста | OS-имя |
|---|---|---|---|---|---|
| Linux | `resolv.conf` + `resolvectl` | `ip route` | `ping -c 1 -W 1` | `ping -c 1 -W 2` | `/etc/os-release` |
| macOS | `scutil --dns` | `route -n get default` | `ping -c 1 -t 1` | `ping -c 1 -W 2000` | `sw_vers` |
| Windows | PowerShell `Get-DnsClientServerAddress` | PowerShell `Get-NetRoute` | `ping -n 1 -w 1000` | `ping -n 1 -w 2000` | PowerShell `Win32_OperatingSystem` |

Сборка: `make cross` → 5 бинарников в `dist/`.

## Лицензия

MIT — см. [LICENSE](LICENSE).

## TODO

Планируемые улучшения — в [`TODOS.md`](TODOS.md).
