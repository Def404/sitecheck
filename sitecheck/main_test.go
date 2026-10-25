package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
		{name: "filename", args: []string{"sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "text", Concurrency: 5}},
		{name: "output format json", args: []string{"--output-format", "json", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "json", Concurrency: 5}},
		{name: "output format json with output", args: []string{"--output-format", "json", "--output", "report.json", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "json", OutputFile: "report.json", Concurrency: 5}},
		{name: "output format json report", args: []string{"--output-format", "json-report", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "json-report", Concurrency: 5}},
		{name: "output format json report with output", args: []string{"--output-format", "json-report", "--output", "report.json", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "json-report", OutputFile: "report.json", Concurrency: 5}},
		{name: "output format text with output", args: []string{"--output-format", "text", "--output", "summary.txt", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "text", OutputFile: "summary.txt", Concurrency: 5}},
		{name: "output format csv", args: []string{"--output-format", "csv", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "csv", Concurrency: 5}},
		{name: "output format csv with output", args: []string{"--output-format", "csv", "--output", "report.csv", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "csv", OutputFile: "report.csv", Concurrency: 5}},
		{name: "output format html", args: []string{"--output-format", "html", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "html", Concurrency: 5}},
		{name: "output format html with output", args: []string{"--output-format", "html", "--output", "report.html", "sites.txt"}, want: Options{Path: "sites.txt", OutputFormat: "html", OutputFile: "report.html", Concurrency: 5}},
		{name: "invalid output format", args: []string{"--output-format", "xml", "sites.txt"}, wantErr: `invalid --output-format "xml": must be text, json, json-report, csv, or html`},
		{name: "output format empty", args: []string{"--output-format", "", "sites.txt"}, wantErr: `invalid --output-format "": must be text, json, json-report, csv, or html`},
		{name: "no args", args: []string{}, wantErr: "filename is required"},
		{name: "output format json without filename", args: []string{"--output-format", "json"}, wantErr: "filename is required"},
		{name: "unknown flag", args: []string{"--unknown", "sites.txt"}, wantErr: "flag provided but not defined: -unknown"},
		{name: "two filenames", args: []string{"a.txt", "b.txt"}, wantErr: "too many arguments"},
		{name: "unknown flag after output format", args: []string{"--output-format", "json", "--unknown"}, wantErr: "flag provided but not defined: -unknown"},
		{name: "empty filename", args: []string{""}, wantErr: "filename is required"},
		{name: "output format json and empty filename", args: []string{"--output-format", "json", ""}, wantErr: "filename is required"},
		{name: "three filenames", args: []string{"a.txt", "b.txt", "c.txt"}, wantErr: "too many arguments"},
		{name: "output format json output without filename", args: []string{"--output-format", "json", "--output", "report.json"}, wantErr: "filename is required"},
		{name: "help", args: []string{"--help"}, want: Options{Help: true}},
		{name: "help with args", args: []string{"--help", "sites.txt"}, want: Options{Help: true}},
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

func TestNewHTTPClientRedirectNotFollowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://example.com/other")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	client := newHTTPClient(Options{})

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("client.Get() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("resp.Body.Close() error = %v, want nil", err)
		}
	})

	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("StatusCode = %d, want %d (redirect must not be followed)", resp.StatusCode, http.StatusMovedPermanently)
	}
}

func TestCheckInterruptedContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := check(ctx, client, server.URL, time.Second, fakeResolver)

	if result.Up {
		t.Errorf("Up = %v, want false", result.Up)
	}
	if result.Err == nil {
		t.Errorf("Err = nil, want cancellation error")
	}
	if result.Reason != "request failed" {
		t.Errorf("Reason = %q, want %q", result.Reason, "request failed")
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

			result := check(context.Background(), client, server.URL, time.Second, fakeResolver)

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

	result := check(context.Background(), client, server.URL, 50*time.Millisecond, fakeResolver)

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

	result := check(context.Background(), client, server.URL, time.Second, failingResolver)

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

func TestBuildReportWithSkipped(t *testing.T) {
	results := []Result{
		{URL: "https://up.example.com", Up: true, Attempts: 1},
		{URL: "https://down.example.com", Up: false, Reason: "DNS", Attempts: 1},
		{URL: "https://skipped1.example.com", Skipped: true},
		{URL: "https://skipped2.example.com", Skipped: true},
	}

	report := buildReport(SystemInfo{}, NetworkInfo{}, results, time.Second)

	if report.Total != 4 {
		t.Errorf("Total = %d, want 4", report.Total)
	}
	if report.UpCount != 1 {
		t.Errorf("UpCount = %d, want 1", report.UpCount)
	}
	if report.DownCount != 1 {
		t.Errorf("DownCount = %d, want 1 (skipped excluded)", report.DownCount)
	}
}

func TestToJSONResultSkipped(t *testing.T) {
	result := Result{
		URL:     "https://skipped.example.com",
		Skipped: true,
	}

	got := toJSONResult(result)
	if !got.Skipped {
		t.Errorf("Skipped = %v, want true", got.Skipped)
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"skipped":true`) {
		t.Errorf("marshaled JSON = %s, want it to contain %q", data, `"skipped":true`)
	}
}

func TestToJSONResultNoSkipped(t *testing.T) {
	result := Result{
		URL:      "https://up.example.com",
		Up:       true,
		Attempts: 1,
	}

	got := toJSONResult(result)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "skipped") {
		t.Errorf("marshaled JSON = %s, want no %q key when not skipped", data, "skipped")
	}
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

func TestFormatPingCell(t *testing.T) {
	tests := []struct {
		name string
		ping PingResult
		want string
	}{
		{name: "not ran", ping: PingResult{}, want: "—"},
		{name: "success", ping: PingResult{Ran: true, Success: true, LatencyMS: 42.5}, want: "OK (42.50 ms)"},
		{name: "failed", ping: PingResult{Ran: true, Success: false}, want: "FAIL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPingCell(tt.ping); got != tt.want {
				t.Errorf("formatPingCell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPingColorEx(t *testing.T) {
	tests := []struct {
		cell string
		want string
	}{
		{cell: "OK (1.00 ms)", want: colorGreen},
		{cell: "FAIL", want: colorRed},
		{cell: "—", want: ""},
	}

	for _, tt := range tests {
		if got := pingColorEx(tt.cell); got != tt.want {
			t.Errorf("pingColorEx(%q) = %q, want %q", tt.cell, got, tt.want)
		}
	}
}

func TestToJSONResultWithPingAndIP(t *testing.T) {
	result := Result{
		URL:        "https://example.com",
		StatusCode: 200,
		Up:         true,
		Duration:   150 * time.Millisecond,
		IPs:        []string{"93.184.216.34", "172.66.147.243"},
		Ping:       PingResult{Ran: true, Success: true, LatencyMS: 35.2},
	}

	jr := toJSONResult(result)

	if jr.IP != "93.184.216.34" {
		t.Errorf("IP = %q, want %q", jr.IP, "93.184.216.34")
	}
	if jr.Ping == nil {
		t.Fatal("Ping is nil, want non-nil")
	}
	if !jr.Ping.Success {
		t.Error("Ping.Success = false, want true")
	}
	if jr.Ping.LatencyMS != 35.2 {
		t.Errorf("Ping.LatencyMS = %f, want 35.2", jr.Ping.LatencyMS)
	}
}

func TestToJSONResultPingNotRan(t *testing.T) {
	result := Result{
		URL:        "https://example.com",
		StatusCode: 200,
		Up:         true,
		Duration:   100 * time.Millisecond,
	}

	jr := toJSONResult(result)

	if jr.Ping != nil {
		t.Errorf("Ping = %v, want nil when not ran", jr.Ping)
	}
	if jr.IP != "" {
		t.Errorf("IP = %q, want empty", jr.IP)
	}
}

func TestToJSONResultPingMarshal(t *testing.T) {
	result := Result{
		URL:        "https://example.com",
		StatusCode: 200,
		Up:         true,
		Duration:   100 * time.Millisecond,
		IPs:        []string{"1.2.3.4"},
		Ping:       PingResult{Ran: true, Success: true, LatencyMS: 12.3},
	}

	data, err := marshalResults([]Result{result})
	if err != nil {
		t.Fatalf("marshalResults() error: %v", err)
	}

	var decoded []JSONResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("len = %d, want 1", len(decoded))
	}
	if decoded[0].IP != "1.2.3.4" {
		t.Errorf("IP = %q, want %q", decoded[0].IP, "1.2.3.4")
	}
	if decoded[0].Ping == nil || !decoded[0].Ping.Success {
		t.Errorf("Ping = %v, want success", decoded[0].Ping)
	}
}

func TestWriteCSV(t *testing.T) {
	results := []Result{
		{
			URL:        "https://example.com",
			StatusCode: 200,
			Up:         true,
			Duration:   150 * time.Millisecond,
			Attempts:   1,
			IPs:        []string{"93.184.216.34"},
			Ping:       PingResult{Ran: true, Success: true, LatencyMS: 18.32},
		},
		{
			URL:        "https://down.example.com",
			StatusCode: 0,
			Up:         false,
			Duration:   5012 * time.Millisecond,
			Attempts:   2,
			Reason:     "DNS",
		},
		{
			URL:     "https://skipped.example.com",
			Skipped: true,
		},
	}

	var buf bytes.Buffer
	if err := writeCSV(&buf, results); err != nil {
		t.Fatalf("writeCSV() error = %v, want nil", err)
	}

	csvReader := csv.NewReader(&buf)
	csvReader.FieldsPerRecord = 10
	records, err := csvReader.ReadAll()
	if err != nil {
		t.Fatalf("csv.ReadAll() error = %v, want nil", err)
	}

	if len(records) != 4 {
		t.Fatalf("len(records) = %d, want 4 (header + 3 rows)", len(records))
	}

	wantHeader := []string{"URL", "Status", "State", "Duration_ms", "Attempts", "Reason", "IP", "Ping_ms", "TLS_expires", "TLS_status"}
	if !slices.Equal(records[0], wantHeader) {
		t.Errorf("header = %v, want %v", records[0], wantHeader)
	}

	// row 0: UP host with IP and ping
	up := records[1]
	if up[0] != "https://example.com" {
		t.Errorf("row0 URL = %q, want %q", up[0], "https://example.com")
	}
	if up[1] != "200" {
		t.Errorf("row0 Status = %q, want %q", up[1], "200")
	}
	if up[2] != "UP" {
		t.Errorf("row0 State = %q, want UP", up[2])
	}
	if up[3] != "150" {
		t.Errorf("row0 Duration_ms = %q, want 150", up[3])
	}
	if up[4] != "1" {
		t.Errorf("row0 Attempts = %q, want 1", up[4])
	}
	if up[6] != "93.184.216.34" {
		t.Errorf("row0 IP = %q, want 93.184.216.34", up[6])
	}
	if up[7] != "18.32" {
		t.Errorf("row0 Ping_ms = %q, want 18.32", up[7])
	}

	// row 1: DOWN host, no IP/ping
	down := records[2]
	if down[2] != "DOWN" {
		t.Errorf("row1 State = %q, want DOWN", down[2])
	}
	if down[5] != "DNS" {
		t.Errorf("row1 Reason = %q, want DNS", down[5])
	}
	if down[6] != "" {
		t.Errorf("row1 IP = %q, want empty", down[6])
	}
	if down[7] != "" {
		t.Errorf("row1 Ping_ms = %q, want empty", down[7])
	}

	// row 2: skipped
	skipped := records[3]
	if skipped[2] != "—" {
		t.Errorf("row2 State = %q, want em-dash for skipped", skipped[2])
	}
	if skipped[3] != "0" {
		t.Errorf("row2 Duration_ms = %q, want 0", skipped[3])
	}
	if skipped[4] != "0" {
		t.Errorf("row2 Attempts = %q, want 0", skipped[4])
	}
}

func TestWriteHTML(t *testing.T) {
	report := Report{
		System:        SystemInfo{Hostname: "host", Username: "alex", OS: "Ubuntu 24.04"},
		Total:         3,
		UpCount:       1,
		DownCount:     1,
		CheckDuration: 5 * time.Second,
		Reasons:       map[string]int{"DNS": 1},
		Results: []Result{
			{URL: "https://example.com", StatusCode: 200, Up: true, Duration: 150 * time.Millisecond, Attempts: 1, IPs: []string{"93.184.216.34"}},
			{URL: "https://down.example.com", Up: false, Duration: 5012 * time.Millisecond, Attempts: 2, Reason: "DNS"},
			{URL: "https://skipped.example.com", Skipped: true},
		},
	}

	var buf bytes.Buffer
	if err := writeHTML(&buf, report, time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("writeHTML() error = %v, want nil", err)
	}

	out := buf.String()

	checks := []string{
		"<!doctype html>",
		"Отчёт",
		"Сформирован",
		"Проверено",
		"Доступно",
		"Недоступно",
		"Результаты",
		"example.com",
		"badge up",
		"badge down",
		"badge", // skipped row
		"93.184.216.34",
		"DNS",
		"Сеть",
		"Система",
		"Причины недоступности",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("HTML output missing %q", want)
		}
	}

	// HTML must escape user-provided values (no raw injection).
	injected := Result{URL: "https://bad.com/?a=<script>", Up: false, Reason: "<b>evil</b>"}
	var buf2 bytes.Buffer
	if err := writeHTML(&buf2, Report{Results: []Result{injected}}, time.Time{}); err != nil {
		t.Fatalf("writeHTML() error = %v, want nil", err)
	}
	if strings.Contains(buf2.String(), "<script>") {
		t.Errorf("HTML output contains unescaped <script> tag")
	}
	if strings.Contains(buf2.String(), "<b>evil</b>") {
		t.Errorf("HTML output contains unescaped <b>evil</b>")
	}
}
