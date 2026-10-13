package main

import (
	"fmt"
	"net"
	"time"
)

func doNslookup(host string) string {
	server := getDNSResolvers()
	addrs, err := net.LookupHost(host)
	if err != nil {
		return "FAIL: " + err.Error()
	}
	if len(addrs) == 0 {
		return "no result"
	}
	ip := addrs[0]
	for _, a := range addrs {
		if parsed := net.ParseIP(a); parsed != nil && parsed.To4() != nil {
			ip = a
			break
		}
	}
	return fmt.Sprintf("%s -> %s", server, ip)
}

func pingGateway(gw string) string {
	if gw == "—" {
		return "шлюз не определён"
	}
	pr := doICMPPing(gw, 1, 2*time.Second)
	if pr == nil {
		return "—"
	}
	return pr.StatusStr()
}
