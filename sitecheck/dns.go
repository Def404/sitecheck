package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func resolveIPs(ctx context.Context, host string) ([]string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve IPs for %q: %w", host, err)
	}

	addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, ip.IP.String())
	}

	return addrs, nil
}

func parseResolvConf(content string) []string {
	var servers []string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}

		if net.ParseIP(fields[1]) != nil {
			servers = append(servers, fields[1])
		}
	}

	return servers
}

var resolvConfPath = "/etc/resolv.conf"

func getLinuxDNSServers() ([]string, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", resolvConfPath, err)
	}

	servers := parseResolvConf(string(data))
	if len(servers) == 0 {
		return nil, fmt.Errorf("no DNS servers found in %s", resolvConfPath)
	}

	return servers, nil
}

func parseMacOSDNSServers(output string) []string {
	var servers []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "nameserver[") {
			continue
		}

		_, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		ip := strings.TrimSpace(rest)
		if net.ParseIP(ip) == nil || seen[ip] {
			continue
		}

		seen[ip] = true
		servers = append(servers, ip)
	}

	return servers
}

func getMacOSDNSServers() ([]string, error) {
	out, err := exec.Command("scutil", "--dns").Output()
	if err != nil {
		return nil, fmt.Errorf("run scutil --dns: %w", err)
	}

	servers := parseMacOSDNSServers(string(out))
	if len(servers) == 0 {
		return nil, fmt.Errorf("no DNS servers found in scutil output")
	}

	return servers, nil
}

func getLinuxResolvedDNS() ([]InterfaceDNS, error) {
	out, err := exec.Command("resolvectl", "status").Output()
	if err != nil {
		return nil, fmt.Errorf("run resolvectl status: %w", err)
	}

	ifaces := parseResolvedDNS(string(out))
	if len(ifaces) == 0 {
		return nil, fmt.Errorf("no DNS servers found in resolvectl output")
	}

	return ifaces, nil
}

func getLinuxDNSInfo() ([]InterfaceDNS, error) {
	ifaces, resolvErr := getLinuxResolvedDNS()
	if resolvErr == nil {
		return ifaces, nil
	}

	servers, err := getLinuxDNSServers()
	if err == nil {
		return []InterfaceDNS{{
			Interface: "resolv.conf",
			Servers:   servers,
		}}, nil
	}

	return nil, errors.Join(resolvErr, fmt.Errorf("fallback to /etc/resolv.conf: %w", err))
}

func parseWindowsDNSServers(output string) ([]string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, fmt.Errorf("windows DNS servers output is empty")
	}

	var raw []string
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("parse windows DNS servers: %w", err)
		}
	case '"':
		var single string
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("parse windows DNS servers: %w", err)
		}
		raw = []string{single}
	default:
		return nil, fmt.Errorf("unexpected windows DNS servers format: %q", trimmed[:1])
	}

	var servers []string
	seen := make(map[string]bool)
	for _, ip := range raw {
		ip = strings.TrimSpace(ip)
		if net.ParseIP(ip) == nil || seen[ip] {
			continue
		}
		seen[ip] = true
		servers = append(servers, ip)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no valid DNS servers found in windows DNS output")
	}

	return servers, nil
}

func getWindowsDNSServers() ([]string, error) {
	script := `Get-DnsClientServerAddress -AddressFamily IPv4 | Select-Object -ExpandProperty ServerAddresses | ConvertTo-Json -Compress`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, fmt.Errorf("run powershell Get-DnsClientServerAddress: %w", err)
	}

	servers, err := parseWindowsDNSServers(string(out))
	if err != nil {
		return nil, fmt.Errorf("get windows DNS servers: %w", err)
	}

	return servers, nil
}

func getDNSServers() ([]string, error) {
	switch runtime.GOOS {
	case "linux":
		return getLinuxDNSServers()
	case "windows":
		return getWindowsDNSServers()
	case "darwin":
		return getMacOSDNSServers()
	default:
		return nil, fmt.Errorf("DNS detection is not supported on %s", runtime.GOOS)
	}
}

type InterfaceDNS struct {
	Interface string   `json:"interface"`
	Servers   []string `json:"servers"`
}

func parseResolvedDNS(output string) []InterfaceDNS {
	var result []InterfaceDNS
	currentIface := ""
	var currentServers []string
	readingDNS := false

	flush := func() {
		if currentIface != "" && len(currentServers) > 0 {
			result = append(result, InterfaceDNS{
				Interface: currentIface,
				Servers:   currentServers,
			})
		}
		currentServers = nil
		readingDNS = false
	}

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Link ") {
			flush()
			currentIface = extractLinkInterfaceName(line)
			continue
		}

		if strings.HasPrefix(trimmed, "Global") {
			flush()
			currentIface = ""
			continue
		}

		if currentIface == "" {
			continue
		}

		if readingDNS {
			if net.ParseIP(trimmed) != nil {
				currentServers = append(currentServers, trimmed)
			} else {
				readingDNS = false
			}
		}

		if strings.HasPrefix(trimmed, "DNS Servers:") {
			readingDNS = true
			_, rest, _ := strings.Cut(trimmed, "DNS Servers:")
			for _, field := range strings.Fields(rest) {
				if net.ParseIP(field) != nil {
					currentServers = append(currentServers, field)
				}
			}
		}
	}

	flush()
	return result
}

func extractLinkInterfaceName(line string) string {
	paren := strings.Index(line, "(")
	closeParen := strings.LastIndex(line, ")")
	if paren < 0 || closeParen < paren {
		return ""
	}
	return line[paren+1 : closeParen]
}
