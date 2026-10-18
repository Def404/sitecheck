package main

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    Options
		wantErr string
	}{
		{name: "filename", args: []string{"sites.txt"}, want: Options{Path: "sites.txt", Concurrency: 5}},
		{name: "json and filename", args: []string{"--json", "sites.txt"}, want: Options{Path: "sites.txt", JSONOutput: true, Concurrency: 5}},
		{name: "json output and filename", args: []string{"--json", "--output", "report.json", "sites.txt"}, want: Options{Path: "sites.txt", JSONOutput: true, OutputFile: "report.json", Concurrency: 5}},
		{name: "json report", args: []string{"--json-report", "sites.txt"}, want: Options{Path: "sites.txt", JSONReport: true, Concurrency: 5}},
		{name: "json report output", args: []string{"--json-report", "--output", "report.json", "sites.txt"}, want: Options{Path: "sites.txt", JSONReport: true, OutputFile: "report.json", Concurrency: 5}},
		{name: "no args", args: []string{}, wantErr: "filename is required"},
		{name: "json without filename", args: []string{"--json"}, wantErr: "filename is required"},
		{name: "unknown flag", args: []string{"--unknown", "sites.txt"}, wantErr: "flag provided but not defined: -unknown"},
		{name: "two filenames", args: []string{"a.txt", "b.txt"}, wantErr: "too many arguments"},
		{name: "unknown flag after json", args: []string{"--json", "--unknown"}, wantErr: "flag provided but not defined: -unknown"},
		{name: "empty filename", args: []string{""}, wantErr: "filename is required"},
		{name: "json and empty filename", args: []string{"--json", ""}, wantErr: "filename is required"},
		{name: "three filenames", args: []string{"a.txt", "b.txt", "c.txt"}, wantErr: "too many arguments"},
		{name: "output without json", args: []string{"--output", "report.json", "sites.txt"}, wantErr: "--output requires --json or --json-report"},
		{name: "json output without filename", args: []string{"--json", "--output", "report.json"}, wantErr: "filename is required"},
		{name: "json and json report", args: []string{"--json", "--json-report", "sites.txt"}, wantErr: "--json and --json-report cannot be used together"},
		{name: "text output", args: []string{"--text-output", "report.txt", "sites.txt"}, want: Options{Path: "sites.txt", TextOutput: "report.txt", Concurrency: 5}},
		{name: "json and text output", args: []string{"--json", "--text-output", "report.txt", "sites.txt"}, wantErr: "--text-output cannot be used with --json or --json-report"},
		{name: "json report and text output", args: []string{"--json-report", "--text-output", "report.txt", "sites.txt"}, wantErr: "--text-output cannot be used with --json or --json-report"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseArgs(%q) = %+v, nil; want error %q", tt.args, got, tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				if got != (Options{}) {
					t.Errorf("parseArgs(%q) = %+v, want zero Options", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%q) error = %v, want nil", tt.args, err)
			}
			if got != tt.want {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

func TestIsUp(t *testing.T) {
	tests := []struct {
		statusCode int
		want       bool
	}{
		{statusCode: 199, want: false},
		{statusCode: 200, want: true},
		{statusCode: 301, want: true},
		{statusCode: 399, want: true},
		{statusCode: 400, want: false},
		{statusCode: 404, want: false},
		{statusCode: 500, want: false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.statusCode), func(t *testing.T) {
			got := isUp(tt.statusCode)
			if got != tt.want {
				t.Errorf("isUp(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{raw: "https://example.com", wantErr: false},
		{raw: "http://example.com", wantErr: false},
		{raw: "example.com", wantErr: true},
		{raw: "htps://example.com", wantErr: true},
		{raw: "https://", wantErr: true},
		{raw: "ftp://example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			u, err := parseURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseURL(%q) = %v, nil; want error", tt.raw, u)
				}
				if u != nil {
					t.Errorf("parseURL(%q) = %v, want nil", tt.raw, u)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseURL(%q) error = %v, want nil", tt.raw, err)
			}
			if u == nil {
				t.Fatalf("parseURL(%q) = nil, want URL", tt.raw)
			}
		})
	}
}

func TestCheckHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantUp     bool
		wantReason string
	}{
		{name: "200 OK", statusCode: 200, wantUp: true, wantReason: ""},
		{name: "301 redirect", statusCode: 301, wantUp: true, wantReason: ""},
		{name: "404 not found", statusCode: 404, wantUp: false, wantReason: "HTTP 404"},
		{name: "500 server error", statusCode: 500, wantUp: false, wantReason: "HTTP 500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusCode := tt.statusCode
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			client := &http.Client{}

			result := check(client, server.URL, time.Second, fakeResolver)

			if result.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", result.StatusCode, tt.statusCode)
			}
			if result.Up != tt.wantUp {
				t.Errorf("Up = %v, want %v", result.Up, tt.wantUp)
			}
			if result.Err != nil {
				t.Errorf("Err = %v, want nil", result.Err)
			}
			if result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}
			if result.Duration <= 0 {
				t.Errorf("Duration = %v, want > 0", result.Duration)
			}
		})
	}
}

func TestCheckTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{}

	result := check(client, server.URL, 50*time.Millisecond, fakeResolver)

	if result.Up {
		t.Errorf("Up = %v, want false", result.Up)
	}
	if result.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", result.StatusCode)
	}
	if result.Err == nil {
		t.Errorf("Err = nil, want timeout error")
	}
	if result.Reason != "timeout" {
		t.Errorf("Reason = %q, want %q", result.Reason, "timeout")
	}
	if result.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", result.Duration)
	}
}

func TestCheckDNSFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{}

	result := check(client, server.URL, time.Second, failingResolver)

	if !result.Up {
		t.Errorf("Up = %v, want true", result.Up)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if result.Err != nil {
		t.Errorf("Err = %v, want nil", result.Err)
	}
	if result.Reason != "" {
		t.Errorf("Reason = %q, want empty", result.Reason)
	}
	if len(result.IPs) != 0 {
		t.Errorf("IPs = %#v, want empty", result.IPs)
	}
	if result.DNSErr == nil {
		t.Error("DNSErr = nil, want error")
	}
}

func TestResolveIPsLocalhost(t *testing.T) {
	got, err := resolveIPs(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatalf("resolveIPs(%q) error = %v, want nil", "127.0.0.1", err)
	}
	if len(got) == 0 {
		t.Fatal("resolveIPs returned no addresses")
	}
	if !slices.Contains(got, "127.0.0.1") {
		t.Errorf("resolveIPs() = %#v, want address %q", got, "127.0.0.1")
	}
}

func TestLoadURLs(t *testing.T) {
	t.Run("parses urls and skips empty lines and comments", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sites.txt")

		content := `# comment

https://example.com
   https://google.com

# internal
https://gitlab.example.com
`
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		got, err := loadURLs(path)
		if err != nil {
			t.Fatalf("loadURLs() error = %v, want nil", err)
		}

		want := []string{
			"https://example.com",
			"https://google.com",
			"https://gitlab.example.com",
		}
		if !slices.Equal(got, want) {
			t.Errorf("loadURLs() = %#v, want %#v", got, want)
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "does-not-exist.txt")

		got, err := loadURLs(path)
		if err == nil {
			t.Fatalf("loadURLs() error = nil, want error")
		}
		if got != nil {
			t.Errorf("loadURLs() = %#v, want nil", got)
		}
	})
}

func TestCountReasons(t *testing.T) {
	results := []Result{
		{Reason: "timeout"},
		{Reason: "DNS"},
		{Reason: "timeout"},
		{Reason: "connection refused"},
		{Reason: "DNS"},
		{Reason: ""},
	}

	got := countReasons(results)
	want := map[string]int{
		"timeout":            2,
		"DNS":                2,
		"connection refused": 1,
	}
	if !maps.Equal(got, want) {
		t.Errorf("countReasons() = %#v, want %#v", got, want)
	}
}

func fakeResolver(_ context.Context, host string) ([]string, error) {
	if host == "example.com" {
		return []string{"192.0.2.10"}, nil
	}

	return nil, fmt.Errorf("unexpected host: %s", host)
}

func failingResolver(_ context.Context, host string) ([]string, error) {
	return nil, fmt.Errorf("resolve %s: dns failure", host)
}

func TestGetVPNInterfaces(t *testing.T) {
	tests := []struct {
		name       string
		interfaces []InterfaceInfo
		want       []InterfaceInfo
	}{
		{
			name: "no VPN interfaces",
			interfaces: []InterfaceInfo{
				{Name: "wlp3s0", IP: "192.168.1.144", Type: "unknown"},
				{Name: "lo", IP: "127.0.0.1", Type: "unknown"},
			},
			want: nil,
		},
		{
			name: "one VPN interface",
			interfaces: []InterfaceInfo{
				{Name: "wlp3s0", IP: "192.168.1.144", Type: "unknown"},
				{Name: "amn0", IP: "10.8.1.17", Type: "vpn"},
				{Name: "docker0", IP: "172.17.0.1", Type: "virtual"},
			},
			want: []InterfaceInfo{
				{Name: "amn0", IP: "10.8.1.17", Type: "vpn"},
			},
		},
		{
			name: "multiple VPN interfaces, order preserved",
			interfaces: []InterfaceInfo{
				{Name: "wlp3s0", IP: "192.168.1.144", Type: "unknown"},
				{Name: "amn0", IP: "10.8.1.17", Type: "vpn"},
				{Name: "eth0", IP: "192.168.0.100", Type: "unknown"},
				{Name: "tun0", IP: "10.9.0.1", Type: "vpn"},
			},
			want: []InterfaceInfo{
				{Name: "amn0", IP: "10.8.1.17", Type: "vpn"},
				{Name: "tun0", IP: "10.9.0.1", Type: "vpn"},
			},
		},
		{
			name:       "empty input",
			interfaces: nil,
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getVPNInterfaces(tt.interfaces)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getVPNInterfaces() = %v, want %v", got, tt.want)
			}
		})
	}
}
