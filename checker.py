#!/usr/bin/env python3
"""Мониторинг доступности списка сайтов.

Проверяет сайты HTTP-запросом (доступен = ответ 2xx/3xx), в параллельных
потоках, по циклу с заданным интервалом. Сообщает об изменениях статуса
по сравнению с предыдущей проверкой. Работает до Ctrl+C.

Примеры:
    python checker.py sites.txt --interval 60
    python checker.py --once https://example.com https://httpbin.org
    python checker.py sites.txt --interval 30 --timeout 5
"""

import argparse
import concurrent.futures
import fcntl
import os
import re
import signal
import socket
import platform
import struct
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime

USER_AGENT = "site-checker/1.0 (+python-urllib)"
MAX_REDIRECTS = 5  # не более 5 редиректов подряд


@dataclass
class Site:
    url: str
    host: str  # хост для отображения (можно домен или IP)


@dataclass
class PingResult:
    ok: bool
    rtts: list[float] = field(default_factory=list)  # задержки успешных ответов, сек
    min_rtt: float | None = None
    max_rtt: float | None = None
    avg_rtt: float | None = None
    lost: int = 0  # сколько пропущено
    error: str | None = None  # причина недоступности (если ok == False)

    def status_str(self) -> str:
        if self.ok:
            avg = self.avg_rtt if self.avg_rtt is not None else 0.0
            lost_part = f" (потеря {self.lost})" if self.lost else ""
            return f"ping OK {avg*1000:6.1f} ms{lost_part}"
        if self.error:
            return f"ping FAIL: {self.error}"
        return "ping FAIL: timeout"


@dataclass
class CheckResult:
    url: str
    host: str
    http: bool  # доступность по HTTP (2xx/3xx)
    http_status: str  # "200", "503", "timeout", "error: ..."
    http_elapsed: float = 0.0
    ping: PingResult | None = None  # None, если ICMP-пинг не включён
    ip: str | None = None  # резолвнутый IP (если удалось)
    headers: dict[str, str] = field(default_factory=dict)  # ключевые HTTP-заголовки ответа
    error_body: str | None = None  # первые 500 символов тела при 4xx/5xx
    timestamp: float = field(default_factory=time.time)

    @property
    def ok(self) -> bool:
        """Главный статус: сайт доступен, если HTTP 2xx/3xx.
        ICMP-пинг — дополнительная информация, не влияет на UP/DOWN."""
        return self.http


def normalize_url(raw: str) -> str | None:
    """Возвращает URL с схемой, если это похоже на URL; иначе None."""
    raw = raw.strip()
    if not raw:
        return None
    # Оставляем строку, если в ней есть "что-то://"; иначе добавляем https://
    if re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*://", raw):
        return raw
    return "https://" + raw


def site_host(url: str) -> str:
    m = re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*://([^/]+)", url)
    return m.group(1) if m else url


def load_sites(args_urls: list[str], file_path: str | None) -> list[Site]:
    """Читает список URL из аргументов и/или файла (по одному на строку)."""
    raws: list[str] = list(args_urls)
    if file_path:
        try:
            with open(file_path, encoding="utf-8") as f:
                raws.extend(f)
        except OSError as e:
            print(f"Ошибка: не удалось прочитать файл {file_path}: {e}", file=sys.stderr)
            sys.exit(1)

    sites: list[Site] = []
    seen: set[str] = set()
    for raw in raws:
        raw = raw.strip()
        if not raw or raw.startswith("#"):
            continue
        url = normalize_url(raw)
        if url is None:
            continue
        if url in seen:
            continue
        seen.add(url)
        sites.append(Site(url=url, host=site_host(url)))
    return sites


# ---- Определение VPN-интерфейса (локальный IP, как в `ip a`) ----

# Понятие «VPN-интерфейс»: имя совпадает с одним из паттернов виртуальных
# туннелей. Docker-мосты (br-*, veth*, docker0) и обычные NIC НЕ считаются.
VPN_NAME_PATTERNS = (
    "tun", "tap", "vpn", "wg", "wireguard", "wires", "ipsec", "xl",
    "gre", "ppp", "utun", "pppoe", "vtun", "vtap",
    "amn",  # Amnezia (amn0, amn1, ...)
)

# Подсети, которые НЕ считаются VPN, даже если имя подходит (Docker/виртуалки).
NON_VPN_SUBNETS = (
    "172.17.", "172.18.", "172.19.", "172.20.", "172.21.", "172.22.",
    "172.23.", "172.24.", "172.25.", "172.26.", "172.27.", "172.28.",
    "172.29.", "172.30.", "172.31.",
    "172.16.",  # Docker по умолчанию
    "127.",     # loopback
)


def _interface_ipv4(name: str) -> str | None:
    """Возвращает IPv4 интерфейса. Пробует `ip -4 -o addr show`, затем ioctl."""
    # Способ 1: ip -4 -o addr show <iface>
    try:
        import subprocess
        out = subprocess.run(
            ["ip", "-4", "-o", "addr", "show", name],
            capture_output=True, text=True, timeout=3,
        )
        if out.returncode == 0 and out.stdout.strip():
            # Формат: "3: wlp3s0    inet 192.168.1.144/24 brd ..."
            m = re.search(r"inet\s+(\d+\.\d+\.\d+\.\d+)", out.stdout)
            if m:
                return m.group(1)
    except (OSError, subprocess.SubprocessError):
        pass

    # Способ 2: ioctl SIOCGIFADDR
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        try:
            packed = struct.pack("256s", name.encode()[:255])
            result = fcntl.ioctl(s, 0x8915, packed)  # SIOCGIFADDR
            return socket.inet_ntoa(result[2:6])
        finally:
            s.close()
    except (OSError, ValueError):
        return None


def detect_vpn() -> tuple[str, list[str]]:
    """Ищет VPN-интерфейс(ы) и их локальные IP.

    Возвращает (название, список IP). Если VPN не найден — ("—", []).
    """
    try:
        interfaces = [name for _, name in socket.if_nameindex()]
    except OSError:
        return ("—", [])

    vpn_ifaces: list[tuple[str, str]] = []  # (name, ip)
    for name in interfaces:
        low = name.lower()
        is_vpn_name = any(p in low for p in VPN_NAME_PATTERNS)
        # Docker-мосты и veth явно исключаем, даже если имя похоже.
        if low.startswith(("docker", "veth", "br-", "cali", "flannel", "cni")):
            continue
        if not is_vpn_name:
            continue
        ip = _interface_ipv4(name)
        if ip and not any(ip.startswith(p) for p in NON_VPN_SUBNETS):
            vpn_ifaces.append((name, ip))

    if not vpn_ifaces:
        return ("—", [])
    # берём первый (обычно один VPN-интерфейс), IP-ы всех совпадающих
    names = sorted({n for n, _ in vpn_ifaces})
    ips = [ip for _, ip in vpn_ifaces]
    return (", ".join(names), ips)


def format_vpn_status(vpn_name: str, ips: list[str]) -> str:
    if ips:
        return f"VPN: {vpn_name}  IP: {', '.join(ips)}"
    return "VPN: не обнаружен"


class _RedirectLimit(urllib.error.HTTPError):
    pass


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Ограничивает число последовательных редиректов."""

    def __init__(self, max_redirects: int):
        super().__init__()
        self._max = max_redirects
        self._count = 0

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        self._count += 1
        if self._count > self._max:
            raise _RedirectLimit(
                code, f"Слишком много редиректов (> {self._max})", None, None, fp
            )
        return super().redirect_request(req, fp, code, msg, headers, newurl)


# ---- ICMP-пинг ----

def _icmp_checksum(data: bytes) -> int:
    """Расчёт контрольной суммы ICMP (RFC 791)."""
    if len(data) % 2:
        data += b"\x00"
    s = sum(struct.unpack("!%dH" % (len(data) // 2), data))
    s = (s >> 16) + (s & 0xFFFF)
    s += s >> 16
    return (~s) & 0xFFFF


def _build_icmp_echo(payload: bytes, ident: int, seq: int) -> bytes:
    """Собирает ICMPv4 Echo Request (тип 8, код 0)."""
    body = struct.pack("!BBHHH", 8, 0, 0, ident, seq) + payload
    csum = _icmp_checksum(body)
    body = struct.pack("!BBHHH", 8, 0, csum, ident, seq) + payload
    return body


def ping_host(host: str, count: int = 3, timeout: float = 2.0) -> PingResult:
    """Отправляет ICMP Echo Request в хост (IPv4) и ждёт Echo Reply.

    Сначала пробует ICMP DGRAM-сокет (Linux без root), затем raw-сокет
    (нужны root/CAP_NET_RAW). Возвращает PingResult со средним RTT.
    """
    try:
        addr = socket.getaddrinfo(host, None, socket.AF_INET)[0][4][0]
    except socket.gaierror as e:
        return PingResult(ok=False, error=f"DNS: {e}")

    # --- открытие сокета: DGRAM (предпочтительно, без root), затем RAW ---
    sock = None
    sock_type = None
    for st in (socket.SOCK_DGRAM, socket.SOCK_RAW):
        try:
            sock = socket.socket(socket.AF_INET, st, socket.IPPROTO_ICMP)
            sock_type = st
            break
        except (PermissionError, OSError):
            sock = None
    if sock is None:
        return PingResult(ok=False, error="нет прав на ICMP-сокет (нужен root/CAP_NET_RAW)")

    ident = os.getpid() & 0xFFFF
    seq_start = 1
    payload = b"qwen-ping-checker"
    rtt_list: list[float] = []
    send_times: dict[int, float] = {}  # seq -> момент отправки (monotonic)
    deadline = time.monotonic() + timeout

    try:
        # отправляем все пакеты с интервалом 0.5с (как утилита ping)
        for seq in range(seq_start, seq_start + count):
            if time.monotonic() >= deadline:
                break
            send_times[seq] = time.monotonic()
            pkt = _build_icmp_echo(payload, ident, seq)
            sock.sendto(pkt, (addr, 0))
            if seq < seq_start + count - 1:
                time.sleep(min(0.5, max(0.0, deadline - time.monotonic())))

        # читаем ответы, пока не собрали все или не истек дедлайн
        got = set()
        while len(got) < count:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                break
            sock.settimeout(remaining)
            try:
                data = sock.recv(1024)
                if sock_type == socket.SOCK_RAW:
                    data = data[20:]  # raw-сокет отдаёт с IP-заголовком
            except socket.timeout:
                break
            if not data or len(data) < 8:
                continue
            icmptype, code, _, r_ident, r_seq = struct.unpack("!BBHHH", data[:8])
            if icmptype != 0:
                continue  # не Echo Reply
            # DGRAM-ICMP: ядро само переписывает ident — фильтруем только по seq.
            # RAW-ICMP: ident остаётся нашим — фильтруем и по ident, и по seq.
            if sock_type == socket.SOCK_RAW and r_ident != ident:
                continue
            if r_seq in send_times:
                rtt = time.monotonic() - send_times[r_seq]
                rtt_list.append(rtt)
                got.add(r_seq)
    except OSError as e:
        return PingResult(ok=False, error=f"socket: {e}")
    finally:
        sock.close()

    lost = count - len(rtt_list)
    if not rtt_list:
        return PingResult(ok=False, error="timeout (нет ответов)")
    return PingResult(
        ok=True,
        rtts=rtt_list,
        min_rtt=min(rtt_list),
        max_rtt=max(rtt_list),
        avg_rtt=sum(rtt_list) / len(rtt_list),
        lost=lost,
    )


def check_site(site: Site, timeout: float, do_ping: bool = False) -> CheckResult:
    """Один HTTP GET (и при do_ping — ICMP-пинг в параллельных потоках).

    ok = статус 2xx или 3xx; при включённом ICMP дополнительно — пинг успешен.
    """
    url = site.url
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": USER_AGENT,
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        },
    )
    opener = urllib.request.build_opener(_NoRedirect(MAX_REDIRECTS))

    def do_http() -> tuple[bool, str, float, dict[str, str], str | None]:
        start = time.monotonic()
        hdrs: dict[str, str] = {}
        err_body: str | None = None
        try:
            with opener.open(req, timeout=timeout) as resp:
                code = resp.getcode()
                # Собираем ключевые заголовки
                for h in ("Server", "X-Powered-By", "Via", "X-Frame-Options", "Content-Type"):
                    val = resp.headers.get(h)
                    if val:
                        hdrs[h] = val
                # TLS-информация
                try:
                    sock = resp.fp.raw._sock  # noqa: SLF001
                    ctx = sock.getpeercert()
                    if ctx:
                        hdrs["TLS"] = f"CN={ctx.get('subjectAltName', '?')}"
                except (AttributeError, OSError):
                    pass
            elapsed = time.monotonic() - start
            return 200 <= code < 400, str(code), elapsed, hdrs, err_body
        except _RedirectLimit:
            return False, f"redirects> {MAX_REDIRECTS}", time.monotonic() - start, hdrs, err_body
        except urllib.error.HTTPError as e:
            # 4xx/5xx — сайт ответил, но не "доступен" по критерию 2xx/3xx.
            for h in ("Server", "X-Powered-By", "Via", "Content-Type"):
                val = e.headers.get(h) if e.headers else None
                if val:
                    hdrs[h] = val
            try:
                body = e.read(500).decode("utf-8", errors="replace")
                err_body = body.strip()[:500]
            except Exception:
                pass
            return False, str(e.code), time.monotonic() - start, hdrs, err_body
        except urllib.error.URLError as e:
            reason = getattr(e, "reason", e)
            if isinstance(reason, socket.timeout):
                return False, "timeout", time.monotonic() - start, hdrs, err_body
            return False, f"error: {reason}", time.monotonic() - start, hdrs, err_body
        except Exception as e:  # noqa: BLE001 — любой непредвиденный сбой = down
            return False, f"error: {e}", time.monotonic() - start, hdrs, err_body

    def do_ping() -> PingResult:
        return ping_host(site.host, count=3, timeout=min(timeout, 2.0))

    if do_ping:
        # HTTP и ICMP параллельно (треды) — не суммируем задержки.
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            http_f = pool.submit(do_http)
            ping_f = pool.submit(do_ping)
            http_ok, http_status, http_elapsed, hdrs, err_body = http_f.result()
            ping_res = ping_f.result()
    else:
        http_ok, http_status, http_elapsed, hdrs, err_body = do_http()
        ping_res = None

    # резолвим IP хоста (best-effort, не влияет на статус)
    ip: str | None = None
    try:
        ip = socket.getaddrinfo(site.host, None, socket.AF_INET)[0][4][0]
    except (socket.gaierror, OSError):
        pass

    return CheckResult(
        url, site.host,
        http=http_ok, http_status=http_status, http_elapsed=http_elapsed,
        ping=ping_res, ip=ip,
        headers=hdrs, error_body=err_body,
    )


def run_check_round(sites: list[Site], timeout: float, do_ping: bool = False) -> list[CheckResult]:
    """Параллельно проверяет все сайты, возвращает результаты в исходном порядке."""
    if not sites:
        return []
    results: list[CheckResult] = [None] * len(sites)  # type: ignore[list-item]
    max_workers = min(32, len(sites))
    with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as pool:
        futs = {
            pool.submit(check_site, site, timeout, do_ping): i for i, site in enumerate(sites)
        }
        for fut in concurrent.futures.as_completed(futs):
            i = futs[fut]
            results[i] = fut.result()
    return results


# ---- Цвета (ANSI) ----
_C_RESET = "\033[0m"
_C_BOLD = "\033[1m"
_C_DIM = "\033[2m"
_C_GREEN = "\033[32m"
_C_RED = "\033[31m"
_C_YELLOW = "\033[33m"
_C_CYAN = "\033[36m"
_C_MAGENTA = "\033[35m"


def _supports_color() -> bool:
    return hasattr(sys.stdout, "isatty") and sys.stdout.isatty()


def _c(text: str, color: str) -> str:
    """Обёртка для цветного текста (без цвета если не tty)."""
    if not _supports_color():
        return text
    return f"{color}{text}{_C_RESET}"


def _strip_ansi(s: str) -> str:
    return re.sub(r"\033\[[0-9;]*m", "", s)


def _vlen(s: str) -> int:
    """Визуальная длина (без ANSI)."""
    return len(_strip_ansi(s))


def _col(s: str, width: int, align: str = "l") -> str:
    """Выравнивает колонку, учитывая ANSI-коды."""
    cur = _vlen(s)
    if cur >= width:
        return s
    pad = " " * (width - cur)
    if align == "r":
        return pad + s
    return s + pad


def format_result(r: CheckResult, ping_enabled: bool = False) -> str:
    flag = _c("✔ UP", _C_GREEN) if r.ok else _c("✘ DOWN", _C_RED)
    flag = _col(flag, 9)
    # Колонка HTTP: статус + время. Длинные ошибки укорачиваем.
    status = r.http_status
    if len(status) > 14:
        status = status[:13] + "…"
    http_col = _col(f"{status:>14}  {r.http_elapsed:6.2f}s", 22)
    if ping_enabled and r.ping is not None:
        ping_col = _col(r.ping.status_str(), 26)
    else:
        ping_col = ""
    host = r.host
    if r.ip:
        host += _c(f" ({r.ip})", _C_DIM)
    parts = [flag, http_col]
    if ping_enabled:
        parts.append(ping_col)
    parts.append(host)
    return "  " + "  ".join(p for p in parts if p)


def render_round(results: list[CheckResult], prev: dict[str, bool], round_no: int, ping_enabled: bool = False) -> str:
    """Возвращает строку-таблицу (без вывода в stdout)."""
    ts = datetime.now().strftime("%H:%M:%S")
    ping_tag = " + ICMP" if ping_enabled else ""

    lines = [format_result(r, ping_enabled=ping_enabled) for r in results]
    if ping_enabled:
        header = _col("Статус", 9) + "  " + _col("HTTP", 19, "l") + "  " + _col("Пинг", 26) + "  " + "Хост"
    else:
        header = _col("Статус", 9) + "  " + _col("HTTP", 19, "l") + "  " + "Хост"
    width = max(_vlen(h) for h in [header] + lines)
    width = max(width, 40)
    sep = "─" * (width + 2)

    out: list[str] = []
    out.append(_c(f"┌{sep}┐", _C_DIM))
    title = _col(f" Проверка #{round_no}  ·  {len(results)} сайт(ов){ping_tag}  ·  {ts} ", width)
    out.append(_c("│", _C_DIM) + title + _c("│", _C_DIM))
    out.append(_c(f"├{sep}┤", _C_DIM))
    out.append(_c("│ ", _C_DIM) + header + _col("", width - _vlen(header)) + _c(" │", _C_DIM))
    out.append(_c(f"├{sep}┤", _C_DIM))
    for line in lines:
        out.append(_c("│ ", _C_DIM) + _col(line, width) + _c(" │", _C_DIM))
    out.append(_c(f"├{sep}┤", _C_DIM))

    ups = sum(1 for r in results if r.ok)
    downs = len(results) - ups
    up_str = _c(f"{ups} доступно", _C_GREEN) if ups else _c(f"{ups} доступно", _C_DIM)
    down_str = _c(f"{downs} недоступно", _C_RED) if downs else _c(f"{downs} недоступно", _C_DIM)
    summary = _col(f" Итого: {up_str}, {down_str}", width)
    out.append(_c("│", _C_DIM) + summary + _c("│", _C_DIM))
    out.append(_c(f"└{sep}┘", _C_DIM))

    changes = []
    for r in results:
        if r.url in prev and prev[r.url] != r.ok:
            direction = "down → up" if r.ok else "up → down"
            color = _C_GREEN if r.ok else _C_RED
            changes.append(f"  {_c('●', color)} {_c(r.host, _C_BOLD)}: {direction}")
    if changes:
        out.append("")
        out.append(_c("  Изменения:", _C_YELLOW))
        out.extend(changes)
    elif round_no > 1:
        out.append(_c("  Изменения: нет", _C_DIM))

    # Причины DOWN (под таблицей)
    down_sites = [r for r in results if not r.ok]
    if down_sites:
        out.append("")
        out.append(_c("  Причины недоступности:", _C_YELLOW))
        for r in down_sites:
            reason = r.http_status
            server = r.headers.get("Server", "")
            if server:
                reason += f"  ({server})"
            if r.error_body:
                first_line = r.error_body.split("\n")[0][:80]
                reason += f"  |  {first_line}"
            out.append(f"    {_c('✘', _C_RED)} {r.host}: {_c(reason, _C_DIM)}")

    return "\n".join(out)


# Счётчик строк предыдущей таблицы для ANSI-очистки
_last_line_count = 0


def dynamic_print(table: str, interactive: bool) -> None:
    """Выводит таблицу. В interactive-режиме — перезаписывает на месте."""
    global _last_line_count
    lines = table.split("\n")
    if interactive:
        # Сдвигаем курсор вверх на N строк и очищаем каждую
        for _ in range(_last_line_count):
            sys.stdout.write("\033[1A\033[2K")
        sys.stdout.write("\n".join(lines) + "\n")
        sys.stdout.flush()
        _last_line_count = len(lines)
    else:
        print(table)


def _get_dns_resolvers() -> str:
    """Возвращает фактические DNS-серверы (upstream, не прокси)."""
    # Пробуем resolvectl (systemd-resolved) — показывает реальные upstream DNS
    try:
        out = subprocess.run(
            ["resolvectl", "status"],
            capture_output=True, text=True, timeout=3,
        )
        if out.returncode == 0:
            servers: list[str] = []
            in_link = False
            for line in out.stdout.splitlines():
                if "DNS Servers:" in line:
                    ips = line.split("DNS Servers:")[1].strip()
                    if ips:
                        servers.append(ips)
            if servers:
                # Берём первый (обычно основной интерфейс)
                return servers[0]
    except (OSError, subprocess.SubprocessError):
        pass

    # Fallback: /etc/resolv.conf
    try:
        with open("/etc/resolv.conf", encoding="ascii") as f:
            for line in f:
                if line.startswith("nameserver"):
                    ip = line.split()[1]
                    # 127.0.0.53 = systemd-resolved прокси, неинформативно
                    if ip != "127.0.0.53":
                        return ip
    except OSError:
        pass
    return "—"


def _get_default_route() -> str:
    """Возвращает маршрут по умолчанию: 'via <gw> dev <iface>'."""
    try:
        out = subprocess.run(
            ["ip", "-4", "route", "show", "default"],
            capture_output=True, text=True, timeout=3,
        )
        if out.returncode == 0 and out.stdout.strip():
            # "default via 192.168.1.1 dev wlp3s0 proto dhcp ..."
            line = out.stdout.strip().splitlines()[0]
            parts = line.split()
            gw = parts[2] if len(parts) > 2 and parts[1] == "via" else "—"
            dev = parts[4] if len(parts) > 4 and parts[3] == "dev" else "—"
            return f"via {gw} dev {dev}"
    except (OSError, subprocess.SubprocessError):
        pass
    return "—"


def _get_gateway_ip() -> str:
    """Извлекает IP шлюза из маршрута по умолчанию."""
    route = _get_default_route()
    if "via " in route:
        return route.split("via ")[1].split()[0]
    return "—"


def _ping_gateway(gw_ip: str, count: int = 2, timeout: float = 2.0) -> str:
    """Быстрый пинг до шлюза, возвращает строку для отчёта."""
    if gw_ip == "—":
        return "шлюз не определён"
    result = ping_host(gw_ip, count=count, timeout=timeout)
    return result.status_str()


def _get_system_info() -> dict[str, str]:
    """Собирает OS, пользователя, hostname и локальный IP."""
    # OS
    try:
        with open("/etc/os-release", encoding="utf-8") as f:
            for line in f:
                if line.startswith("PRETTY_NAME="):
                    os_name = line.split("=", 1)[1].strip().strip('"')
                    break
            else:
                os_name = f"{platform.system()} {platform.release()}"
    except OSError:
        os_name = f"{platform.system()} {platform.release()}"

    # Локальный IP (не VPN, не Docker)
    local_ip = "—"
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("10.255.255.255", 1))
        local_ip = s.getsockname()[0]
        s.close()
    except OSError:
        pass

    return {
        "os": os_name,
        "user": os.environ.get("USER", os.environ.get("LOGNAME", "—")),
        "host": platform.node(),
        "ip": local_ip,
    }


def _nslookup(host: str) -> str:
    """Возвращает краткий результат nslookup: 'server -> ip' или ошибку."""
    try:
        out = subprocess.run(
            ["nslookup", host],
            capture_output=True, text=True, timeout=5,
        )
        # Формат вывода (успех):
        #   Server:	127.0.0.53
        #   Address: 127.0.0.53#53
        #   Non-authoritative answer:
        #   Name:	gitlab.corpsoft24.ru
        #   Address: 46.235.53.9
        # Формат (ошибка):
        #   ** server can't find host: NXDOMAIN
        server = ""
        result_ip = ""
        seen_name = False
        error_msg = ""
        for line in out.stdout.splitlines():
            stripped = line.strip()
            if stripped.startswith("Server:"):
                server = stripped.split(":", 1)[1].strip()
            elif stripped.startswith("Name:"):
                seen_name = True
            elif stripped.startswith("Address:") and seen_name:
                result_ip = stripped.split(":", 1)[1].strip()
                break
            elif stripped.startswith("**"):
                error_msg = stripped[2:].strip()[:80]
        if result_ip:
            return f"{server} -> {result_ip}"
        if error_msg:
            return f"FAIL: {error_msg}"
        return "no result"
    except (OSError, subprocess.SubprocessError):
        return "nslookup недоступен"


def generate_report(
    results: list[CheckResult],
    round_no: int,
    sysinfo: dict[str, str],
    vpn_name: str,
    vpn_ips: list[str],
    prev: dict[str, bool],
    session_start: float,
    ping_enabled: bool = False,
) -> str:
    """Генерирует plain-text отчёт для техподдержки (без ANSI)."""
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    uptime_s = time.time() - session_start
    uptime_str = f"{uptime_s:.0f}s" if uptime_s < 120 else f"{uptime_s / 60:.1f}min"
    lines: list[str] = []
    W = 70
    lines.append("=" * W)
    lines.append(f"  Отчёт мониторинга сайтов")
    lines.append(f"  Проверка #{round_no}  ·  {ts}  ·  время сессии: {uptime_str}")
    lines.append("=" * W)

    # --- Сеть ---
    gw = _get_gateway_ip()
    lines.append("")
    lines.append("[СЕТЬ]")
    lines.append(f"  Локальный IP:   {sysinfo['ip']}")
    lines.append(f"  Шлюз:           {gw}  ({_ping_gateway(gw)})")
    lines.append(f"  Маршрут:        {_get_default_route()}")
    lines.append(f"  DNS-резолверы:  {_get_dns_resolvers()}")
    lines.append(f"  VPN:            {format_vpn_status(vpn_name, vpn_ips)}")

    # --- Система ---
    lines.append("")
    lines.append("[СИСТЕМА]")
    lines.append(f"  OS:           {sysinfo['os']}")
    lines.append(f"  Пользователь: {sysinfo['user']}@{sysinfo['host']}")

    # --- Результаты ---
    ups = sum(1 for r in results if r.ok)
    downs = len(results) - ups
    lines.append("")
    lines.append(f"[РЕЗУЛЬТАТЫ]  {ups} UP  /  {downs} DOWN  из {len(results)}")
    lines.append("-" * W)

    for r in results:
        status = "UP " if r.ok else "DOWN"
        ip_str = f"  ->  {r.ip}" if r.ip else ""
        lines.append(f"  [{status}]  {r.http_status:>10s}  {r.http_elapsed:6.2f}s  {r.url}{ip_str}")

        # DNS (nslookup) — без порта
        dns_host = r.host.split(":")[0]
        dns_result = _nslookup(dns_host)
        lines.append(f"           DNS: {dns_result}")

        # Детали для DOWN сайтов
        if not r.ok:
            reason = r.http_status
            if r.headers:
                server = r.headers.get("Server", "")
                if server:
                    reason += f"  (server: {server})"
            if r.error_body:
                # Первая строка тела (часто там причина)
                first_line = r.error_body.split("\n")[0][:120]
                reason += f"  |  {first_line}"
            lines.append(f"           причина: {reason}")

        # TLS для UP сайтов (коротко)
        if r.ok and r.headers.get("TLS"):
            tls = r.headers["TLS"]
            # Извлекаем первый SAN: "(('DNS', 'example.com'), ...)" -> "example.com"
            m = re.search(r"'DNS',\s*'([^']+)'", tls)
            if m:
                lines.append(f"           TLS: {m.group(1)}")

    # --- Пинг (если включён) ---
    if ping_enabled:
        lines.append("")
        lines.append("[ICMP-ПИНГ]")
        for r in results:
            if r.ping is not None:
                lines.append(f"  {r.host:40s}  {r.ping.status_str()}")

    # --- Изменения ---
    changes = []
    for r in results:
        if r.url in prev and prev[r.url] != r.ok:
            direction = "down -> up" if r.ok else "up -> down"
            changes.append(f"  {r.host}: {direction}")
    if changes:
        lines.append("")
        lines.append("[ИЗМЕНЕНИЯ]")
        lines.extend(changes)

    lines.append("")
    lines.append("=" * W)
    lines.append("")
    return "\n".join(lines)


def _log_write(log_file: str, text: str) -> None:
    """Дописывает текст в лог-файл."""
    with open(log_file, "a", encoding="utf-8") as f:
        f.write(text)


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Мониторинг доступности списка сайтов (HTTP GET, 2xx/3xx = доступен).",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "Примеры:\n"
            "  python checker.py sites.txt --interval 60\n"
            "  python checker.py --once https://example.com https://httpbin.org\n"
            "  python checker.py sites.txt --interval 30 --timeout 5 --icmp\n"
        ),
    )
    parser.add_argument(
        "urls",
        nargs="*",
        help="URL(ы) для проверки (можно несколько).",
    )
    parser.add_argument(
        "-f", "--file",
        help="Файл со списком URL (по одному на строку, '#' — комментарий).",
    )
    parser.add_argument(
        "-i", "--interval",
        type=float,
        default=60.0,
        help="Интервал между проверками, секунд (по умолчанию 60).",
    )
    parser.add_argument(
        "-t", "--timeout",
        type=float,
        default=10.0,
        help="Таймаут HTTP-запроса, секунд (по умолчанию 10).",
    )
    parser.add_argument(
        "--once",
        action="store_true",
        help="Один раз, без цикла мониторинга.",
    )
    parser.add_argument(
        "--icmp",
        action="store_true",
        help="Дополнительно ICMP-пинг (3 пакета) параллельно с HTTP. "
             "Нужны права на ICMP-сокет (root/CAP_NET_RAW) или поддержка ICMP DGRAM.",
    )
    parser.add_argument(
        "--log",
        metavar="FILE",
        help="Сохранять отчёт в файл (plain text, без ANSI). Подходит для техподдержки.",
    )
    args = parser.parse_args()

    if not args.urls and not args.file:
        parser.error("Укажите URL(ы) аргументами или файл с -f/--file.")

    sites = load_sites(args.urls, args.file)
    if not sites:
        print("Ошибки: нет ни одного URL для проверки.", file=sys.stderr)
        sys.exit(1)

    if args.interval <= 0:
        print("Ошибки: --interval должен быть > 0.", file=sys.stderr)
        sys.exit(1)

    # Грассульное завершение по Ctrl+C — флаг, а не исключение внутри цикла.
    stop = {"flag": False}

    def _handle_sigint(signum, frame):
        stop["flag"] = True
        print("\nОстановлено по Ctrl+C.", file=sys.stderr)

    signal.signal(signal.SIGINT, _handle_sigint)

    ping_mode = " + ICMP" if args.icmp else ""
    sysinfo = _get_system_info()
    print()
    print(_c(f"  ⬢ Мониторинг сайтов", _C_BOLD))
    print(_c(f"  {len(sites)} сайт(ов)  ·  интервал {args.interval:.0f}s  ·  "
             f"таймаут {args.timeout:.0f}s{ping_mode}", _C_DIM))
    print(_c(f"  {sysinfo['os']}  ·  {sysinfo['user']}@{sysinfo['host']}  ·  "
             f"{sysinfo['ip']}", _C_DIM))
    if args.icmp:
        print(_c(f"  ICMP-пинг: 3 пакета (дополнительная информация)", _C_DIM))
    vpn_name, vpn_ips = detect_vpn()
    print(_c(f"  {format_vpn_status(vpn_name, vpn_ips)}", _C_DIM))
    if args.log:
        print(_c(f"  Лог: {args.log}", _C_DIM))
    print(_c(f"  Остановка: Ctrl+C", _C_DIM))
    print()

    # Заголовок лог-файла
    if args.log:
        _log_write(
            args.log,
            f"{'=' * 70}\n"
            f"Мониторинг сайтов  ·  запуск {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n"
            f"{'=' * 70}\n\n",
        )

    prev: dict[str, bool] = {}
    round_no = 0
    session_start = time.time()
    interactive = not args.once and _supports_color()
    while not stop["flag"]:
        round_no += 1
        results = run_check_round(sites, args.timeout, do_ping=args.icmp)
        table = render_round(results, prev, round_no, ping_enabled=args.icmp)
        dynamic_print(table, interactive)
        # Лог-файл
        if args.log:
            report = generate_report(
                results, round_no, sysinfo, vpn_name, vpn_ips, prev,
                session_start, ping_enabled=args.icmp,
            )
            _log_write(args.log, report)
        prev = {r.url: r.ok for r in results}

        if args.once or stop["flag"]:
            break

        # ждём интервал кусочками, чтобы быстро реагировать на Ctrl+C
        wait_left = args.interval
        while wait_left > 0 and not stop["flag"]:
            step = min(0.5, wait_left)
            time.sleep(step)
            wait_left -= step

    # финальный свод
    if interactive:
        global _last_line_count
        for _ in range(_last_line_count):
            sys.stdout.write("\033[1A\033[2K")
        sys.stdout.flush()
        _last_line_count = 0
    final = prev
    up = sum(1 for v in final.values() if v)
    down = len(final) - up
    up_str = _c(f"{up} доступно", _C_GREEN)
    down_str = _c(f"{down} недоступно", _C_RED) if down else _c(f"{down} недоступно", _C_DIM)
    print()
    print(_c(f"  ═══ Финал: {up_str}, {down_str} из {len(sites)} ═══", _C_BOLD))


if __name__ == "__main__":
    main()
