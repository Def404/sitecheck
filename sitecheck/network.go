package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type InterfaceInfo struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Type string `json:"type"`
}

type NetworkInfo struct {
	PrimaryIP     string          `json:"primary_ip"`
	LocalIP       string          `json:"local_ip"`
	Gateway       GatewayInfo     `json:"gateway"`
	LocalIPs      []string        `json:"local_ips"`
	LANError      string          `json:"lan_error,omitempty"`
	DNSServers    []string        `json:"dns_servers,omitempty"`
	DNSError      string          `json:"dns_error,omitempty"`
	DNSInterfaces []InterfaceDNS  `json:"dns_interfaces,omitempty"`
	VPNInterfaces []InterfaceInfo `json:"vpn_interfaces,omitempty"`
}

func getNetworkInfo() (NetworkInfo, error) {
	info := NetworkInfo{}

	primaryIP, err := getPrimaryLocalIP()
	if err != nil {
		return info, fmt.Errorf("get primary local IP: %w", err)
	}
	info.PrimaryIP = primaryIP

	localIPs, err := getLocalIPs()
	if err != nil {
		return info, fmt.Errorf("get local IPs: %w", err)
	}
	info.LocalIPs = localIPs

	interfaces, err := getIPv4Interfaces()
	if err != nil {
		return info, fmt.Errorf("get IPv4 interfaces: %w", err)
	}
	info.VPNInterfaces = getVPNInterfaces(interfaces)

	localIP, gateway, err := getLANInfo(interfaces)
	if err != nil {
		info.LANError = err.Error()
	} else {
		info.LocalIP = localIP
		info.Gateway = gateway
	}

	if runtime.GOOS == "linux" {
		if ifaces, err := getLinuxDNSInfo(); err != nil {
			info.DNSError = err.Error()
		} else {
			info.DNSInterfaces = ifaces
			info.DNSServers = collectDNSServers(ifaces)
		}
	} else {
		dnsServers, err := getDNSServers()
		if err != nil {
			info.DNSError = err.Error()
		} else {
			info.DNSServers = dnsServers
		}
	}

	return info, nil
}

func collectDNSServers(ifaces []InterfaceDNS) []string {
	var servers []string
	seen := make(map[string]bool)

	for _, iface := range ifaces {
		for _, ip := range iface.Servers {
			if !seen[ip] {
				seen[ip] = true
				servers = append(servers, ip)
			}
		}
	}

	return servers
}

func classifyInterface(name string) string {
	if isVPNInterface(name) {
		return "vpn"
	}

	if isDockerInterface(name) {
		return "virtual"
	}

	return "unknown"
}

func getVPNInterfaces(interfaces []InterfaceInfo) []InterfaceInfo {
	var vpn []InterfaceInfo
	for _, iface := range interfaces {
		if iface.Type == "vpn" {
			vpn = append(vpn, iface)
		}
	}
	return vpn
}

func getIPv4Interfaces() ([]InterfaceInfo, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("get network interfaces: %w", err)
	}

	var ifaces []InterfaceInfo

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("get addresses for %s: %w", iface.Name, err)
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			ifaces = append(ifaces, InterfaceInfo{
				Name: iface.Name,
				IP:   ip.String(),
				Type: classifyInterface(iface.Name),
			})
		}
	}

	return ifaces, nil
}

func getLocalIPs() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("get network interfaces: %w", err)
	}

	var localIPs []string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("get addresses for %s: %w", iface.Name, err)
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			localIPs = append(localIPs, ip.String())
		}
	}

	return localIPs, nil
}

func findInterfaceIP(interfaces []InterfaceInfo, interfaceName string) (string, error) {
	for _, iface := range interfaces {
		if iface.Name == interfaceName {
			return iface.IP, nil
		}
	}

	return "", fmt.Errorf("interface %s not found", interfaceName)
}

func findInterfaceIPByIndex(index int) (string, error) {
	iface, err := net.InterfaceByIndex(index)
	if err != nil {
		return "", fmt.Errorf("find interface by index %d: %w", index, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("get addresses for interface index %d: %w", index, err)
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP.To4()
		if ip == nil {
			continue
		}

		return ip.String(), nil
	}

	return "", fmt.Errorf("no IPv4 address found for interface index %d", index)
}

func isDockerInterface(name string) bool {
	prefixes := []string{"docker", "br-", "veth"}
	lower := strings.ToLower(name)

	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

func isVPNInterface(name string) bool {
	prefixes := []string{"tun", "tap", "wg", "wireguard", "utun", "vpn", "amn"}
	lower := strings.ToLower(name)

	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

func getLinuxRoutes() (string, error) {
	out, err := exec.Command("ip", "-4", "route", "show").Output()
	if err != nil {
		return "", fmt.Errorf("run ip -4 route show: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

type GatewayInfo struct {
	IP        string `json:"ip"`
	Interface string `json:"interface"`
}

func parseDefaultGateway(routes string) (GatewayInfo, error) {
	for _, line := range strings.Split(routes, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "default" {
			continue
		}

		var gwIP, iface string
		for i := 1; i < len(fields)-1; i++ {
			if fields[i] == "via" {
				gwIP = fields[i+1]
			} else if fields[i] == "dev" {
				iface = fields[i+1]
			}
		}

		if gwIP == "" || iface == "" {
			return GatewayInfo{}, fmt.Errorf("default route found but missing via or dev: %s", line)
		}

		return GatewayInfo{IP: gwIP, Interface: iface}, nil
	}

	return GatewayInfo{}, errors.New("default route not found")
}

func getLANInfo(interfaces []InterfaceInfo) (string, GatewayInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return getLinuxLANInfo(interfaces)
	case "windows":
		return getWindowsLANInfo()
	case "darwin":
		return getMacOSLANInfo(interfaces)
	default:
		return "", GatewayInfo{}, fmt.Errorf("LAN detection is not supported on %s", runtime.GOOS)
	}
}

func getMacOSDefaultRoute() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", fmt.Errorf("run route -n get default: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

func parseMacOSGateway(output string) (GatewayInfo, error) {
	var gwIP, iface string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "gateway:") {
			gwIP = strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		} else if strings.HasPrefix(line, "interface:") {
			iface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
		}
	}

	if gwIP == "" {
		return GatewayInfo{}, errors.New("gateway not found in route output")
	}
	if iface == "" {
		return GatewayInfo{}, errors.New("interface not found in route output")
	}

	return GatewayInfo{IP: gwIP, Interface: iface}, nil
}

func getMacOSLANInfo(interfaces []InterfaceInfo) (string, GatewayInfo, error) {
	output, err := getMacOSDefaultRoute()
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("get macOS default route: %w", err)
	}

	gateway, err := parseMacOSGateway(output)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("parse macOS gateway: %w", err)
	}

	lanIP, err := findInterfaceIP(interfaces, gateway.Interface)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("find interface IP: %w", err)
	}

	return lanIP, gateway, nil
}

func getWindowsDefaultRoutes() (string, error) {
	script := `Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Select-Object InterfaceIndex, NextHop, RouteMetric | ConvertTo-Json -Compress`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", fmt.Errorf("run powershell Get-NetRoute: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

type WindowsRoute struct {
	InterfaceIndex int    `json:"InterfaceIndex"`
	NextHop        string `json:"NextHop"`
	RouteMetric    int    `json:"RouteMetric"`
}

func selectWindowsDefaultRoute(routes []WindowsRoute) (WindowsRoute, error) {
	var best *WindowsRoute

	for i := range routes {
		if routes[i].NextHop == "" || routes[i].NextHop == "0.0.0.0" {
			continue
		}

		if best == nil || routes[i].RouteMetric < best.RouteMetric {
			best = &routes[i]
		}
	}

	if best == nil {
		return WindowsRoute{}, errors.New("no valid default route found")
	}

	return *best, nil
}

func parseWindowsRoutes(output string) ([]WindowsRoute, error) {
	trimmed := strings.TrimSpace(output)

	if trimmed == "" {
		return nil, errors.New("windows routes output is empty")
	}

	switch trimmed[0] {
	case '[':
		var routes []WindowsRoute
		if err := json.Unmarshal([]byte(trimmed), &routes); err != nil {
			return nil, fmt.Errorf("parse windows routes array: %w", err)
		}
		if len(routes) == 0 {
			return nil, errors.New("no default routes found in windows output")
		}
		return routes, nil
	case '{':
		var route WindowsRoute
		if err := json.Unmarshal([]byte(trimmed), &route); err != nil {
			return nil, fmt.Errorf("parse windows route object: %w", err)
		}
		return []WindowsRoute{route}, nil
	default:
		return nil, fmt.Errorf("unexpected JSON format: starts with %q", trimmed[0])
	}
}

func getWindowsLANInfo() (string, GatewayInfo, error) {
	output, err := getWindowsDefaultRoutes()
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("get windows default routes: %w", err)
	}

	routes, err := parseWindowsRoutes(output)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("parse windows routes: %w", err)
	}

	best, err := selectWindowsDefaultRoute(routes)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("select windows default route: %w", err)
	}

	lanIP, err := findInterfaceIPByIndex(best.InterfaceIndex)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("find interface IP by index: %w", err)
	}

	gateway := GatewayInfo{
		IP:        best.NextHop,
		Interface: strconv.Itoa(best.InterfaceIndex),
	}

	return lanIP, gateway, nil
}

func getLinuxLANInfo(interfaces []InterfaceInfo) (string, GatewayInfo, error) {
	routes, err := getLinuxRoutes()
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("get linux routes: %w", err)
	}

	gateway, err := parseDefaultGateway(routes)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("parse default gateway: %w", err)
	}

	lanIP, err := findInterfaceIP(interfaces, gateway.Interface)
	if err != nil {
		return "", GatewayInfo{}, fmt.Errorf("find interface IP: %w", err)
	}

	return lanIP, gateway, nil
}

func getPrimaryLocalIP() (string, error) {
	conn, err := net.Dial("udp", "1.1.1.1:53")
	if err != nil {
		return "", fmt.Errorf("dial 1.1.1.1:53 to detect primary local IP: %w", err)
	}

	defer conn.Close()

	udpAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected local address type %T", conn.LocalAddr())
	}

	ip := udpAddr.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("primary local IP is not IPv4: %s", udpAddr.IP)
	}

	return ip.String(), nil
}
