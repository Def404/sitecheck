package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

type SystemInfo struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
}

func getSystemInfo() (SystemInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return SystemInfo{}, fmt.Errorf("get hostname: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return SystemInfo{}, fmt.Errorf("get current user: %w", err)
	}

	OS, err := getOSName()
	if err != nil {
		return SystemInfo{}, fmt.Errorf("get OS name: %w", err)
	}

	return SystemInfo{
		Hostname: hostname,
		Username: currentUser.Username,
		OS:       OS,
	}, nil
}

func getOSName() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return getLinuxOSName()
	case "windows":
		return getWindowsOSName()
	case "darwin":
		return getMacOSName()
	default:
		return "", fmt.Errorf("unknown platform: %s", runtime.GOOS)
	}
}

func getLinuxOSName() (string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("read /etc/os-release: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			value := strings.TrimPrefix(line, "PRETTY_NAME=")
			return strings.Trim(value, `"`), nil
		}
	}

	return "", errors.New("PRETTY_NAME not found in /etc/os-release")
}

func getMacOSName() (string, error) {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return "", fmt.Errorf("run sw_vers: %w", err)
	}

	version := strings.TrimSpace(string(out))

	if version == "" {
		return "", errors.New("sw_vers returned empty version")
	}

	return fmt.Sprintf("macOS %s", version), nil
}

func getWindowsOSName() (string, error) {
	cmd := `Get-CimInstance Win32_OperatingSystem | Select-Object -ExpandProperty Caption`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("run powershell: %w", err)
	}

	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", errors.New("powershell returned empty OS name")
	}

	return name, nil
}
