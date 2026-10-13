package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

type SystemInfo struct {
	OS   string
	User string
	Host string
	IP   string
}

func getSystemInfo() SystemInfo {
	return SystemInfo{
		OS:   getOSName(),
		User: getCurrentUser(),
		Host: getHostname(),
		IP:   getLocalIP(),
	}
}

func getOSName() string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/etc/os-release")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				}
			}
		}
		return "Linux " + runtime.GOARCH
	case "darwin":
		out, err := exec.Command("sw_vers", "-productName").Output()
		if err == nil {
			return strings.TrimSpace(string(out)) + " (macOS)"
		}
		return "macOS"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

func getCurrentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func getLocalIP() string {
	// Connect to a public IP (no actual packet sent) to find the local IP
	conn, err := net.Dial("udp", "10.255.255.255:80")
	if err != nil {
		return "—"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// --- VPN Detection ---

var vpnNamePatterns = []string{
	"tun", "tap", "vpn", "wg", "wireguard", "wires", "ipsec",
	"gre", "ppp", "utun", "pppoe", "vtun", "vtap", "amn",
}

var nonVPNPrefixed = []string{
	"docker", "veth", "br-", "cali", "flannel", "cni",
}

func detectVPN() (string, []string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "—", nil
	}

	var names []string
	var ips []string

	for _, iface := range ifaces {
		name := strings.ToLower(iface.Name)

		// Skip Docker/virtual
		isNonVPN := false
		for _, p := range nonVPNPrefixed {
			if strings.HasPrefix(name, p) {
				isNonVPN = true
				break
			}
		}
		if isNonVPN {
			continue
		}

		// Check VPN pattern
		isVPN := false
		for _, p := range vpnNamePatterns {
			if strings.Contains(name, p) {
				isVPN = true
				break
			}
		}
		if !isVPN {
			continue
		}

		// Get IP
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
		names = append(names, iface.Name)
	}

	if len(names) == 0 {
		return "—", nil
	}
	return strings.Join(names, ", "), ips
}

func formatVPNStatus(name string, ips []string) string {
	if name == "—" {
		return "VPN: не обнаружен"
	}
	if len(ips) > 0 {
		return fmt.Sprintf("VPN: %s  IP: %s", name, strings.Join(ips, ", "))
	}
	return "VPN: " + name
}

// --- Network info ---

func getDNSResolvers() string {
	// Linux: resolvectl
	if out, err := exec.Command("resolvectl", "status").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "DNS Servers:") {
				parts := strings.SplitN(line, "DNS Servers:", 2)
				if len(parts) > 1 {
					s := strings.TrimSpace(parts[1])
					if s != "" && s != "127.0.0.53" {
						return s
					}
				}
			}
		}
	}
	// Fallback: /etc/resolv.conf
	data, err := os.ReadFile("/etc/resolv.conf")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "nameserver") {
				parts := strings.Fields(line)
				if len(parts) > 1 && parts[1] != "127.0.0.53" {
					return parts[1]
				}
			}
		}
	}
	return "—"
}

func getDefaultRoute() string {
	switch runtime.GOOS {
	case "linux":
		out, err := exec.Command("ip", "-4", "route", "show", "default").Output()
		if err != nil {
			return "—"
		}
		line := strings.TrimSpace(string(out))
		if line == "" {
			return "—"
		}
		// "default via 192.168.1.1 dev wlp3s0"
		parts := strings.Fields(line)
		gw, dev := "—", "—"
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "via" {
				gw = parts[i+1]
			}
			if parts[i] == "dev" {
				dev = parts[i+1]
			}
		}
		return fmt.Sprintf("via %s dev %s", gw, dev)
	case "windows":
		out, err := exec.Command("route", "print", "0.0.0.0").Output()
		if err != nil {
			return "—"
		}
		// Parse "0.0.0.0  0.0.0.0  192.168.1.1  ..."
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "0.0.0.0") {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					return fmt.Sprintf("via %s", parts[2])
				}
			}
		}
		return "—"
	case "darwin":
		out, err := exec.Command("route", "-n", "get", "default").Output()
		if err != nil {
			return "—"
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "gateway:") {
				return fmt.Sprintf("via %s", strings.TrimSpace(strings.SplitN(line, "gateway:", 2)[1]))
			}
		}
		return "—"
	}
	return "—"
}

func getGatewayIP() string {
	route := getDefaultRoute()
	// Extract IP after "via "
	if idx := strings.Index(route, "via "); idx >= 0 {
		rest := route[idx+4:]
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "—"
}
