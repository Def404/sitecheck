package main

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// VPNClientInfo содержит информацию о VPN-клиенте.
type VPNClientInfo struct {
	Type            string `json:"type"`
	ClientName      string `json:"client_name"`
	Detected        bool   `json:"detected"`
	DetectionMethod string `json:"detection_method,omitempty"`
}

// getVPNClientInfo определяет тип VPN-клиента по запущенным процессам.
func getVPNClientInfo(ctx context.Context) VPNClientInfo {
	switch runtime.GOOS {
	case "linux":
		return getLinuxVPNClient(ctx)
	case "darwin":
		return getMacOSVPNClient(ctx)
	case "windows":
		return getWindowsVPNClient(ctx)
	default:
		return VPNClientInfo{Type: "unknown", Detected: false}
	}
}

// getLinuxVPNClient определяет VPN-клиент на Linux.
func getLinuxVPNClient(ctx context.Context) VPNClientInfo {
	out, err := exec.CommandContext(ctx, "ps", "aux").Output()
	if err != nil {
		return VPNClientInfo{Type: "unknown", Detected: false}
	}

	return parseLinuxVPNProcesses(string(out))
}

// parseLinuxVPNProcesses парсит вывод ps aux и определяет VPN-клиент.
func parseLinuxVPNProcesses(output string) VPNClientInfo {
	processes := []string{
		"openvpn",
		"wireguard",
		"wg-quick",
		"amnezia",
	}

	for _, process := range processes {
		if strings.Contains(output, process) {
			return VPNClientInfo{
				Type:            mapProcessToType(process),
				ClientName:      process,
				Detected:        true,
				DetectionMethod: "process",
			}
		}
	}

	return VPNClientInfo{Type: "unknown", Detected: false}
}

// getMacOSVPNClient определяет VPN-клиент на macOS.
func getMacOSVPNClient(ctx context.Context) VPNClientInfo {
	out, err := exec.CommandContext(ctx, "ps", "aux").Output()
	if err != nil {
		return VPNClientInfo{Type: "unknown", Detected: false}
	}

	return parseMacOSVPNProcesses(string(out))
}

// parseMacOSVPNProcesses парсит вывод ps aux и определяет VPN-клиент.
func parseMacOSVPNProcesses(output string) VPNClientInfo {
	processes := []struct {
		keyword    string
		clientType string
	}{
		{"openvpn", "openvpn"},
		{"wireguard", "wireguard"},
		{"amnezia", "amnezia"},
	}

	outputLower := strings.ToLower(output)

	for _, p := range processes {
		if strings.Contains(outputLower, p.keyword) {
			return VPNClientInfo{
				Type:            p.clientType,
				ClientName:      p.keyword,
				Detected:        true,
				DetectionMethod: "process",
			}
		}
	}

	return VPNClientInfo{Type: "unknown", Detected: false}
}

// getWindowsVPNClient определяет VPN-клиент на Windows.
func getWindowsVPNClient(ctx context.Context) VPNClientInfo {
	script := `Get-Process | Select-Object -ExpandProperty Name`
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return VPNClientInfo{Type: "unknown", Detected: false}
	}

	return parseWindowsVPNProcesses(string(out))
}

// parseWindowsVPNProcesses парсит список процессов и определяет VPN-клиент.
func parseWindowsVPNProcesses(output string) VPNClientInfo {
	processes := []struct {
		keyword    string
		clientType string
	}{
		{"openvpn", "openvpn"},
		{"wireguard", "wireguard"},
		{"wg", "wireguard"},
		{"amnezia", "amnezia"},
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		name := strings.TrimSpace(strings.ToLower(line))
		for _, p := range processes {
			if strings.Contains(name, p.keyword) {
				return VPNClientInfo{
					Type:            p.clientType,
					ClientName:      name,
					Detected:        true,
					DetectionMethod: "process",
				}
			}
		}
	}

	return VPNClientInfo{Type: "unknown", Detected: false}
}

// mapProcessToType возвращает нормализованный тип VPN по имени процесса.
func mapProcessToType(process string) string {
	switch {
	case strings.Contains(process, "openvpn"):
		return "openvpn"
	case strings.Contains(process, "wireguard") || strings.Contains(process, "wg-quick"):
		return "wireguard"
	case strings.Contains(process, "amnezia"):
		return "amnezia"
	default:
		return "unknown"
	}
}
