package main

import (
	"testing"
)

func TestParseLinuxVPNProcesses(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantType string
		wantDet  bool
	}{
		{
			name:     "no VPN processes",
			output:   "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\nroot 1 0.0 0.1 16844 1234 ? Ss 10:00 0:00 systemd",
			wantType: "unknown",
			wantDet:  false,
		},
		{
			name:     "openvpn detected",
			output:   "root 1234 0.1 0.2 5678 1234 ? Sl 10:00 0:01 openvpn --config /etc/openvpn/client.conf",
			wantType: "openvpn",
			wantDet:  true,
		},
		{
			name:     "wireguard detected",
			output:   "root 5678 0.0 0.1 2345 678 ? S 10:01 0:00 wg-quick up wg0",
			wantType: "wireguard",
			wantDet:  true,
		},
		{
			name:     "amnezia detected",
			output:   "user 9999 0.5 1.2 123456 789 ? Sl 10:02 0:05 amnezia-wg",
			wantType: "amnezia",
			wantDet:  true,
		},
		{
			name:     "amnezia + openvpn → amnezia (wrapper wins)",
			output:   "root 1234 0.1 0.2 5678 1234 ? Sl 10:00 0:01 openvpn --config /etc/amnezia/client.conf\nuser 9999 0.5 1.2 123456 789 ? Sl 10:02 0:05 /usr/bin/amnezia",
			wantType: "amnezia",
			wantDet:  true,
		},
		{
			name:     "multiple processes, first match wins",
			output:   "root 1234 0.1 0.2 5678 1234 ? Sl 10:00 0:01 openvpn --config /etc/openvpn/client.conf\nroot 5678 0.0 0.1 2345 678 ? S 10:01 0:00 wg-quick up wg0",
			wantType: "openvpn",
			wantDet:  true,
		},
		{
			name:     "partial word match should not trigger",
			output:   "user 1111 0.1 0.1 3456 789 ? S 10:03 0:00 my-openvpn-manager",
			wantType: "openvpn",
			wantDet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLinuxVPNProcesses(tt.output)
			if got.Type != tt.wantType {
				t.Errorf("parseLinuxVPNProcesses().Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Detected != tt.wantDet {
				t.Errorf("parseLinuxVPNProcesses().Detected = %v, want %v", got.Detected, tt.wantDet)
			}
			if tt.wantDet && got.DetectionMethod != "process" {
				t.Errorf("parseLinuxVPNProcesses().DetectionMethod = %v, want 'process'", got.DetectionMethod)
			}
		})
	}
}

func TestParseMacOSVPNProcesses(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantType string
		wantDet  bool
	}{
		{
			name:     "no VPN processes",
			output:   "USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\nroot 1 0.0 0.1 16844 1234 ?? Ss 10:00 0:00 /sbin/launchd",
			wantType: "unknown",
			wantDet:  false,
		},
		{
			name:     "openvpn detected",
			output:   "_openvpn 1234 0.5 1.0 234567 89 ? S 10:00 0:02 /usr/local/bin/openvpn --config client.ovpn",
			wantType: "openvpn",
			wantDet:  true,
		},
		{
			name:     "amnezia detected",
			output:   "user 5678 0.2 0.5 45678 123 ? S 10:01 0:01 /Applications/AmneziaVPN.app/Contents/MacOS/AmneziaVPN",
			wantType: "amnezia",
			wantDet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMacOSVPNProcesses(tt.output)
			if got.Type != tt.wantType {
				t.Errorf("parseMacOSVPNProcesses().Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Detected != tt.wantDet {
				t.Errorf("parseMacOSVPNProcesses().Detected = %v, want %v", got.Detected, tt.wantDet)
			}
		})
	}
}

func TestParseWindowsVPNProcesses(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantType string
		wantDet  bool
	}{
		{
			name:     "no VPN processes",
			output:   "System\nsmss.exe\ncsrss.exe\nwininit.exe",
			wantType: "unknown",
			wantDet:  false,
		},
		{
			name:     "openvpn detected",
			output:   "System\nsmss.exe\nopenvpn.exe\nservices.exe",
			wantType: "openvpn",
			wantDet:  true,
		},
		{
			name:     "wireguard detected via wg.exe",
			output:   "System\nsmss.exe\nwg.exe\nservices.exe",
			wantType: "wireguard",
			wantDet:  true,
		},
		{
			name:     "case insensitive",
			output:   "System\nOpenVPN.exe\nservices.exe",
			wantType: "openvpn",
			wantDet:  true,
		},
		{
			name:     "amnezia detected",
			output:   "System\namnezia.exe\nservices.exe",
			wantType: "amnezia",
			wantDet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWindowsVPNProcesses(tt.output)
			if got.Type != tt.wantType {
				t.Errorf("parseWindowsVPNProcesses().Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Detected != tt.wantDet {
				t.Errorf("parseWindowsVPNProcesses().Detected = %v, want %v", got.Detected, tt.wantDet)
			}
		})
	}
}

func TestMapProcessToType(t *testing.T) {
	tests := []struct {
		process string
		want    string
	}{
		{"openvpn", "openvpn"},
		{"openvpn-tunnel", "openvpn"},
		{"wireguard", "wireguard"},
		{"wg-quick", "wireguard"},
		{"amnezia", "amnezia"},
		{"amnezia-wg", "amnezia"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.process, func(t *testing.T) {
			got := mapProcessToType(tt.process)
			if got != tt.want {
				t.Errorf("mapProcessToType(%q) = %v, want %v", tt.process, got, tt.want)
			}
		})
	}
}
