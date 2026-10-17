package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type PingResult struct {
	Ran       bool    `json:"ran"`
	Success   bool    `json:"success"`
	LatencyMS float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

func runLinuxPing(ip string) (string, error) {
	cmd := exec.Command("ping", "-n", "-c", "1", "-W", "2", ip)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("run ping -n -c 1 -W 2 %s: %w", ip, err)
	}
	return text, nil
}

func isPingExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func parseLinuxPingLatency(output string) (float64, error) {
	_, after, ok := strings.Cut(output, "time=")
	if !ok {
		return 0, errors.New("ping output: time= not found")
	}
	fields := strings.Fields(after)
	if len(fields) == 0 {
		return 0, errors.New("ping output: no value after time=")
	}
	latency, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse ping latency %q: %w", fields[0], err)
	}
	return latency, nil
}

func pingLinuxGateway(ip string) (PingResult, error) {
	output, err := runLinuxPing(ip)
	if err != nil {
		if isPingExitError(err) {
			msg := strings.TrimSpace(output)
			if msg == "" {
				msg = err.Error()
			}
			return PingResult{Ran: true, Success: false, Error: msg}, nil
		}
		return PingResult{}, fmt.Errorf("ping gateway %s: %w", ip, err)
	}
	latency, err := parseLinuxPingLatency(output)
	if err != nil {
		return PingResult{}, fmt.Errorf("parse ping gateway %s: %w", ip, err)
	}
	return PingResult{Ran: true, Success: true, LatencyMS: latency}, nil
}

func runMacOSPing(ip string) (string, error) {
	cmd := exec.Command("ping", "-c", "1", "-W", "2000", ip)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("run ping -c 1 -W 2000 %s: %w", ip, err)
	}
	return text, nil
}

func parseMacOSPingLatency(output string) (float64, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "64 bytes from") {
			continue
		}
		_, after, ok := strings.Cut(line, "time=")
		if !ok {
			continue
		}
		after = strings.TrimSpace(after)
		after, _ = strings.CutSuffix(after, " ms")
		latency, err := strconv.ParseFloat(after, 64)
		if err != nil {
			return 0, fmt.Errorf("parse ping latency %q: %w", after, err)
		}
		return latency, nil
	}
	return 0, errors.New("ping output: no reply line found")
}

func pingMacOSGateway(ip string) (PingResult, error) {
	output, err := runMacOSPing(ip)
	if err != nil {
		if isPingExitError(err) {
			msg := strings.TrimSpace(output)
			if msg == "" {
				msg = err.Error()
			}
			return PingResult{Ran: true, Success: false, Error: msg}, nil
		}
		return PingResult{}, fmt.Errorf("ping gateway %s: %w", ip, err)
	}
	latency, err := parseMacOSPingLatency(output)
	if err != nil {
		return PingResult{}, fmt.Errorf("parse ping gateway %s: %w", ip, err)
	}
	return PingResult{Ran: true, Success: true, LatencyMS: latency}, nil
}

func runWindowsPing(ip string) (string, error) {
	cmd := exec.Command("ping", "-n", "1", "-w", "2000", ip)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("run ping -n 1 -w 2000 %s: %w", ip, err)
	}
	return text, nil
}

func parseWindowsPingLatency(output string) (float64, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Reply from") && !strings.HasPrefix(line, "Ответ") {
			continue
		}
		// Handle English: "time=12ms", "time<1ms"
		// Handle Russian: "время=12мс", "время<1мс"
		after := ""
		found := false
		for _, sep := range []string{"time=", "time<", "время=", "время<"} {
			_, a, ok := strings.Cut(line, sep)
			if ok {
				after = a
				found = true
				break
			}
		}
		if !found {
			continue
		}
		after = strings.TrimSpace(after)
		after, _ = strings.CutSuffix(after, "ms")
		after, _ = strings.CutSuffix(after, "мс")
		after = strings.TrimSpace(after)
		latency, err := strconv.ParseFloat(after, 64)
		if err != nil {
			return 0, fmt.Errorf("parse ping latency %q: %w", after, err)
		}
		return latency, nil
	}
	return 0, errors.New("ping output: no reply line found")
}

func pingWindowsGateway(ip string) (PingResult, error) {
	output, err := runWindowsPing(ip)
	if err != nil {
		if isPingExitError(err) {
			msg := strings.TrimSpace(output)
			if msg == "" {
				msg = err.Error()
			}
			return PingResult{Ran: true, Success: false, Error: msg}, nil
		}
		return PingResult{}, fmt.Errorf("ping gateway %s: %w", ip, err)
	}
	latency, err := parseWindowsPingLatency(output)
	if err != nil {
		return PingResult{}, fmt.Errorf("parse ping gateway %s: %w", ip, err)
	}
	return PingResult{Ran: true, Success: true, LatencyMS: latency}, nil
}

func pingGateway(ip string) (PingResult, error) {
	switch runtime.GOOS {
	case "linux":
		return pingLinuxGateway(ip)
	case "darwin":
		return pingMacOSGateway(ip)
	case "windows":
		return pingWindowsGateway(ip)
	default:
		return PingResult{}, fmt.Errorf("ping is not supported on %s", runtime.GOOS)
	}
}
