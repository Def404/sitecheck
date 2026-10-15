package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

func getTLSExpiration(ctx context.Context, host string, port string) (time.Time, error) {
	dialer := &tls.Dialer{
		Config: &tls.Config{
			ServerName: host,
		},
	}

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return time.Time{}, fmt.Errorf("TLS connection to %s: %w", host, err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return time.Time{}, fmt.Errorf("TLS connection to %s: unexpected connection type %T", host, conn)
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, fmt.Errorf("TLS connection to %s: no peer certificates", host)
	}

	return certs[0].NotAfter, nil
}

func tlsStatus(expiresAt time.Time, err error) string {
	if err != nil && isExpiredCertificate(err) {
		return "EXPIRED"
	}

	if err != nil {
		return "ERROR"
	}

	if expiresAt.IsZero() {
		return "UNKNOWN"
	}

	if time.Until(expiresAt) < 7*24*time.Hour {
		return "WARNING"
	}

	return "OK"
}

func daysUntilExpiration(expiresAt time.Time) int {
	remaining := time.Until(expiresAt)

	days := int(remaining / (24 * time.Hour))

	if days < 0 {
		return 0
	}

	return days
}
