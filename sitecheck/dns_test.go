package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParseResolvConf(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "single nameserver",
			content: "nameserver 8.8.8.8\n",
			want:    []string{"8.8.8.8"},
		},
		{
			name: "multiple nameservers, order preserved",
			content: "nameserver 8.8.8.8\n" +
				"nameserver 8.8.4.4\n" +
				"nameserver 1.1.1.1\n",
			want: []string{"8.8.8.8", "8.8.4.4", "1.1.1.1"},
		},
		{
			name: "ignores comments and blank lines",
			content: "# comment line\n" +
				"\n" +
				"nameserver 8.8.8.8\n" +
				"# another comment\n" +
				"   \n" +
				"nameserver 8.8.4.4\n",
			want: []string{"8.8.8.8", "8.8.4.4"},
		},
		{
			name: "ignores non-nameserver directives",
			content: "search example.com\n" +
				"options ndots:0\n" +
				"nameserver 9.9.9.9\n" +
				"search internal.example.com\n",
			want: []string{"9.9.9.9"},
		},
		{
			name: "ignores nameserver without valid IP",
			content: "nameserver\n" +
				"nameserver notanip\n" +
				"nameserver 9.9.9.9\n",
			want: []string{"9.9.9.9"},
		},
		{
			name:    "handles leading whitespace before directive",
			content: "  nameserver 1.0.0.1\n",
			want:    []string{"1.0.0.1"},
		},
		{
			name:    "handles tabs as field separator",
			content: "nameserver\t1.0.0.2\n",
			want:    []string{"1.0.0.2"},
		},
		{
			name:    "empty content",
			content: "",
			want:    nil,
		},
		{
			name:    "only comments",
			content: "# only comments here\n# and nothing else\n",
			want:    nil,
		},
		{
			name:    "only blank lines",
			content: "\n\n\n",
			want:    nil,
		},
		{
			name: "systemd-resolved stub still parsed",
			content: "nameserver 127.0.0.53\n" +
				"options edns0\n" +
				"nameserver 8.8.8.8\n",
			want: []string{"127.0.0.53", "8.8.8.8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResolvConf(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseResolvConf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLinuxDNSServers(t *testing.T) {
	t.Run("reads and parses resolv.conf", func(t *testing.T) {
		content := "nameserver 8.8.8.8\nnameserver 1.1.1.1\n"
		path := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		orig := resolvConfPath
		resolvConfPath = path
		defer func() { resolvConfPath = orig }()

		got, err := getLinuxDNSServers()
		if err != nil {
			t.Fatalf("getLinuxDNSServers() error = %v, want nil", err)
		}
		want := []string{"8.8.8.8", "1.1.1.1"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("getLinuxDNSServers() = %v, want %v", got, want)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		orig := resolvConfPath
		resolvConfPath = filepath.Join(t.TempDir(), "nonexistent")
		defer func() { resolvConfPath = orig }()

		_, err := getLinuxDNSServers()
		if err == nil {
			t.Fatal("getLinuxDNSServers() = nil, want error")
		}
		if !strings.Contains(err.Error(), "read ") {
			t.Errorf("error = %q, want context about read", err)
		}
	})

	t.Run("no nameserver entries", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(path, []byte("search example.com\noptions ndots:0\n"), 0600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		orig := resolvConfPath
		resolvConfPath = path
		defer func() { resolvConfPath = orig }()

		got, err := getLinuxDNSServers()
		if err == nil {
			t.Fatalf("getLinuxDNSServers() = %v, nil; want error", got)
		}
		if !strings.Contains(err.Error(), "no DNS servers found") {
			t.Errorf("error = %q, want 'no DNS servers found'", err)
		}
	})
}

func TestParseMacOSDNSServers(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "two servers, example from spec",
			output: "    Domain : example.com\n" +
				"    Search Domain\n" +
				"      example.com\n" +
				"    Nameserver\n" +
				"      nameserver[0] : 192.168.1.1\n" +
				"      nameserver[1] : 1.1.1.1\n",
			want: []string{"192.168.1.1", "1.1.1.1"},
		},
		{
			name:   "single server",
			output: "  nameserver[0] : 8.8.8.8\n",
			want:   []string{"8.8.8.8"},
		},
		{
			name: "duplicates removed, first occurrence order kept",
			output: "  nameserver[0] : 8.8.8.8\n" +
				"  nameserver[1] : 1.1.1.1\n" +
				"  nameserver[2] : 8.8.8.8\n",
			want: []string{"8.8.8.8", "1.1.1.1"},
		},
		{
			name: "ignores non-nameserver lines",
			output: "  Domain : corp.local\n" +
				"  Search Domain\n" +
				"    corp.local\n" +
				"  Nameserver\n" +
				"    nameserver[0] : 10.0.0.1\n",
			want: []string{"10.0.0.1"},
		},
		{
			name: "ignores invalid IP in nameserver line",
			output: "  nameserver[0] : notanip\n" +
				"  nameserver[1] : 9.9.9.9\n",
			want: []string{"9.9.9.9"},
		},
		{
			name: "ignores nameserver line without colon",
			output: "  nameserver[0]\n" +
				"  nameserver[1] : 2.2.2.2\n",
			want: []string{"2.2.2.2"},
		},
		{
			name: "handles no space after colon",
			output: "  nameserver[0]:192.168.1.1\n" +
				"  nameserver[1]: 1.1.1.1\n",
			want: []string{"192.168.1.1", "1.1.1.1"},
		},
		{
			name:   "handles extra spaces after colon",
			output: "  nameserver[0] :    8.8.8.8\n",
			want:   []string{"8.8.8.8"},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "only search domains, no nameservers",
			output: "  Domain : example.com\n  Search Domain\n    example.com\n",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMacOSDNSServers(tt.output)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseMacOSDNSServers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseWindowsDNSServers(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    []string
		wantErr string
	}{
		{
			name:   "JSON array of two IPs",
			output: `["192.168.1.1","1.1.1.1"]`,
			want:   []string{"192.168.1.1", "1.1.1.1"},
		},
		{
			name:   "single JSON string",
			output: `"8.8.8.8"`,
			want:   []string{"8.8.8.8"},
		},
		{
			name:   "array with surrounding whitespace",
			output: `  ["9.9.9.9","10.0.0.1"]  `,
			want:   []string{"9.9.9.9", "10.0.0.1"},
		},
		{
			name:   "duplicates removed, first occurrence order kept",
			output: `["8.8.8.8","1.1.1.1","8.8.8.8"]`,
			want:   []string{"8.8.8.8", "1.1.1.1"},
		},
		{
			name:   "invalid IP in array",
			output: `["8.8.8.8","notanip","1.1.1.1"]`,
			want:   []string{"8.8.8.8", "1.1.1.1"},
		},
		{
			name:    "all entries invalid",
			output:  `["notanip","alsoinvalid"]`,
			wantErr: "no valid DNS servers found",
		},
		{
			name:    "empty input",
			output:  "",
			wantErr: "empty",
		},
		{
			name:    "whitespace-only input",
			output:  "   ",
			wantErr: "empty",
		},
		{
			name:    "malformed JSON array",
			output:  `[not valid]`,
			wantErr: "parse windows DNS servers",
		},
		{
			name:    "malformed JSON string",
			output:  `"`,
			wantErr: "parse windows DNS servers",
		},
		{
			name:    "unexpected format",
			output:  `hello`,
			wantErr: "unexpected windows DNS servers format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWindowsDNSServers(tt.output)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseWindowsDNSServers() = %v, nil; want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWindowsDNSServers() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseWindowsDNSServers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDNSServers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("getDNSServers() on %s is platform-specific; this test covers the linux branch", runtime.GOOS)
	}

	got, err := getDNSServers()
	if err != nil {
		t.Fatalf("getDNSServers() error = %v, want nil", err)
	}
	if len(got) == 0 {
		t.Fatal("getDNSServers() returned empty list, want at least one DNS server")
	}
}

func TestGetLinuxResolvedDNS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("getLinuxResolvedDNS() is Linux-only")
	}

	got, err := getLinuxResolvedDNS()
	if err != nil {
		t.Fatalf("getLinuxResolvedDNS() error = %v, want nil", err)
	}
	if len(got) == 0 {
		t.Fatal("getLinuxResolvedDNS() returned empty list, want at least one interface")
	}

	for _, iface := range got {
		if iface.Interface == "" {
			t.Errorf("interface name is empty: %+v", iface)
		}
		if len(iface.Servers) == 0 {
			t.Errorf("interface %s has no servers: %+v", iface.Interface, iface)
		}
	}
}

func TestGetLinuxDNSInfo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("getLinuxDNSInfo() is Linux-only")
	}

	got, err := getLinuxDNSInfo()
	if err != nil {
		t.Fatalf("getLinuxDNSInfo() error = %v, want nil", err)
	}
	if len(got) == 0 {
		t.Fatal("getLinuxDNSInfo() returned empty list, want at least one interface")
	}

	for _, iface := range got {
		if iface.Interface == "" {
			t.Errorf("interface name is empty: %+v", iface)
		}
		if len(iface.Servers) == 0 {
			t.Errorf("interface %s has no servers: %+v", iface.Interface, iface)
		}
	}
}

func TestParseResolvedDNS(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []InterfaceDNS
	}{
		{
			name: "spec example with multiple links",
			output: "Global\n" +
				"    resolv.conf mode: stub\n" +
				"\n" +
				"Link 3 (wlp3s0)\n" +
				"    Current DNS Server: 192.168.1.1\n" +
				"           DNS Servers: 192.168.1.1\n" +
				"\n" +
				"Link 5 (docker0)\n" +
				"    Current Scopes: none\n" +
				"\n" +
				"Link 28 (amn0)\n" +
				"    Current DNS Server: 1.0.0.1\n" +
				"           DNS Servers: 1.1.1.1 1.0.0.1\n" +
				"            DNS Domain: ~.\n",
			want: []InterfaceDNS{
				{Interface: "wlp3s0", Servers: []string{"192.168.1.1"}},
				{Interface: "amn0", Servers: []string{"1.1.1.1", "1.0.0.1"}},
			},
		},
		{
			name:   "single link with one server",
			output: "Link 1 (eth0)\n    DNS Servers: 10.0.0.1\n",
			want:   []InterfaceDNS{{Interface: "eth0", Servers: []string{"10.0.0.1"}}},
		},
		{
			name:   "link without DNS servers is skipped",
			output: "Link 1 (eth0)\n    DNS Servers: 10.0.0.1\nLink 2 (docker0)\n    Current Scopes: none\n",
			want:   []InterfaceDNS{{Interface: "eth0", Servers: []string{"10.0.0.1"}}},
		},
		{
			name:   "ignores Global section",
			output: "Global\n    resolv.conf mode: stub\nLink 1 (eth0)\n    DNS Servers: 10.0.0.1\n",
			want:   []InterfaceDNS{{Interface: "eth0", Servers: []string{"10.0.0.1"}}},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "no link sections",
			output: "Global\n    resolv.conf mode: stub\n",
			want:   nil,
		},
		{
			name: "global after link resets interface",
			output: "Link 3 (wlp3s0)\n" +
				"    DNS Servers: 192.168.1.1\n" +
				"\n" +
				"Global\n" +
				"    DNS Servers: 8.8.8.8\n",
			want: []InterfaceDNS{
				{Interface: "wlp3s0", Servers: []string{"192.168.1.1"}},
			},
		},
		{
			name:   "link without space in prefix is not a section header",
			output: "Linked to 10.0.0.1\nLink 1 (eth0)\n    DNS Servers: 10.0.0.1\n",
			want:   []InterfaceDNS{{Interface: "eth0", Servers: []string{"10.0.0.1"}}},
		},
		{
			name: "DNS servers wrap to next line",
			output: "Link 1 (eth0)\n" +
				"    DNS Servers: 10.0.0.1\n" +
				"        10.0.0.2\n",
			want: []InterfaceDNS{
				{Interface: "eth0", Servers: []string{"10.0.0.1", "10.0.0.2"}},
			},
		},
		{
			name: "DNS servers wrap across multiple lines",
			output: "Link 1 (eth0)\n" +
				"    DNS Servers: 10.0.0.1\n" +
				"        10.0.0.2\n" +
				"        10.0.0.3\n",
			want: []InterfaceDNS{
				{Interface: "eth0", Servers: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
			},
		},
		{
			name: "DNS Domain after wrapped servers stops accumulation",
			output: "Link 1 (eth0)\n" +
				"    DNS Servers: 10.0.0.1\n" +
				"        10.0.0.2\n" +
				"    DNS Domain: ~.\n",
			want: []InterfaceDNS{
				{Interface: "eth0", Servers: []string{"10.0.0.1", "10.0.0.2"}},
			},
		},
		{
			name: "new link after wrapped servers starts fresh",
			output: "Link 1 (eth0)\n" +
				"    DNS Servers: 10.0.0.1\n" +
				"        10.0.0.2\n" +
				"Link 2 (eth1)\n" +
				"    DNS Servers: 192.168.1.1\n",
			want: []InterfaceDNS{
				{Interface: "eth0", Servers: []string{"10.0.0.1", "10.0.0.2"}},
				{Interface: "eth1", Servers: []string{"192.168.1.1"}},
			},
		},
		{
			name: "global after wrapped servers starts fresh",
			output: "Link 1 (eth0)\n" +
				"    DNS Servers: 10.0.0.1\n" +
				"        10.0.0.2\n" +
				"Global\n",
			want: []InterfaceDNS{
				{Interface: "eth0", Servers: []string{"10.0.0.1", "10.0.0.2"}},
			},
		},
		{
			name: "two DNS Servers blocks in same link merge into one",
			output: "Link 28 (amn0)\n" +
				"    DNS Servers: 1.1.1.1\n" +
				"    DNS Domain: ~.\n" +
				"    DNS Servers: 1.0.0.1\n",
			want: []InterfaceDNS{
				{Interface: "amn0", Servers: []string{"1.1.1.1", "1.0.0.1"}},
			},
		},
		{
			name: "spec: two links, wrap, domain, second DNS Servers",
			output: "Link 3 (wlp3s0)\n" +
				"    DNS Servers: 192.168.1.1\n" +
				"\n" +
				"Link 28 (amn0)\n" +
				"    DNS Servers: 1.1.1.1\n" +
				"                 1.0.0.1\n" +
				"    DNS Domain: ~.\n" +
				"    DNS Servers: 9.9.9.9\n",
			want: []InterfaceDNS{
				{Interface: "wlp3s0", Servers: []string{"192.168.1.1"}},
				{Interface: "amn0", Servers: []string{"1.1.1.1", "1.0.0.1", "9.9.9.9"}},
			},
		},
		{
			name: "consecutive DNS Servers without intervening non-IP",
			output: "Link 28 (amn0)\n" +
				"    DNS Servers: 1.1.1.1\n" +
				"    DNS Servers: 9.9.9.9\n",
			want: []InterfaceDNS{
				{Interface: "amn0", Servers: []string{"1.1.1.1", "9.9.9.9"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseResolvedDNS(tt.output)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseResolvedDNS() = %v, want %v", got, tt.want)
			}
		})
	}
}
