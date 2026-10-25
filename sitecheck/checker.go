package main

import (
	"fmt"
	"net/url"
	"time"
)

type Result struct {
	URL          string
	StatusCode   int
	Up           bool
	Duration     time.Duration
	Err          error
	Reason       string
	Retryable    bool
	Attempts     int
	Skipped      bool
	IPs          []string
	DNSErr       error
	Ping         PingResult
	TLSExpiresAt time.Time
	TLSErr       error
}

func (r Result) State() string {
	if r.Up {
		return "UP"
	}

	return "DOWN"
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse URL %q: %w", raw, err)
	}

	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid URL: %q", raw)
	}

	return u, nil
}

func isUp(statusCode int) bool {
	return statusCode >= 200 && statusCode < 400
}
