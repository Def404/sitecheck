# checker-site

Однократная проверка списка сайтов: HTTP GET, DNS, срок TLS-сертификата.

Код — пакет [`sitecheck`](sitecheck/). Модуль Go (`go.mod`) лежит в корне репозитория. Сборка и тесты: `make`.

## Запуск

```bash
make build
./dist/sitecheck sitecheck/site.txt
./dist/sitecheck --json-report --output report.json sitecheck/site.txt
SITECHECK_TIMEOUT=10s ./dist/sitecheck --concurrency 8 --retries 1 sitecheck/site.txt
```

Код выхода `1`, если хотя бы один сайт недоступен или аргументы неверны.

## Аргументы

| Флаг | Описание | По умолчанию |
|---|---|---|
| файл (позиционный) | список URL, один на строку; `#` — комментарий | — |
| `--concurrency` | параллельные проверки | 5 |
| `--retries` | повторы после сбоя | 0 |
| `--json` | JSON-массив результатов | выкл |
| `--json-report` | JSON-отчёт (система, сеть, итоги) | выкл |
| `--output FILE` | записать JSON в файл (вместе с `--json` или `--json-report`) | stdout |
| `--text-output FILE` | продублировать текстовый отчёт в файл | stdout |
| `--version` | версия сборки | — |

Таймаут HTTP: переменная `SITECHECK_TIMEOUT` (по умолчанию `5s`).

## Формат списка

Один URL на строку, схема `http` или `https`. Строки с `#` и пустые строки пропускаются.
