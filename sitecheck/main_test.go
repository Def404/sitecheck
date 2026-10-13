package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

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

func TestValidateURL(t *testing.T) {
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
			err := validateURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateURL(%q) = nil, want error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateURL(%q) = %v, want nil", tt.raw, err)
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			client := &http.Client{
				Timeout: time.Second,
			}

			result := check(client, server.URL)

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{
		Timeout: 50 * time.Millisecond,
	}

	result := check(client, server.URL)

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
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
