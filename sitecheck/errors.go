package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"syscall"
)

func isTimeout(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error

	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	return false
}

func isDNSError(err error) bool {
	if err == nil {
		return false
	}

	var dnsErr *net.DNSError

	if errors.As(err, &dnsErr) {
		return true
	}

	return false
}

func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	var unknownAuth x509.UnknownAuthorityError
	if errors.As(err, &unknownAuth) {
		return true
	}

	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		return true
	}

	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return true
	}

	return false
}

func isExpiredCertificate(err error) bool {
	if err == nil {
		return false
	}

	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		return true
	}

	return false
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if isTLSError(err) {
		return false
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTemporary
	}

	return true
}

func statusReason(statusCode int) string {
	if isUp(statusCode) {
		return ""
	}

	return fmt.Sprintf("HTTP %d", statusCode)
}

func errorReason(err error) string {
	if err == nil {
		return ""
	}

	if isTimeout(err) {
		return "timeout"
	}

	if isDNSError(err) {
		return "DNS"
	}

	if isConnectionRefused(err) {
		return "connection refused"
	}

	if isTLSError(err) {
		return "TLS"
	}

	return "request failed"
}
