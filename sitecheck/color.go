package main

import (
	"os"
	"strconv"

	"golang.org/x/term"
)

var useColor = term.IsTerminal(int(os.Stdout.Fd()))

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

func colorize(code, s string) string {
	if !useColor {
		return s
	}
	return code + s + colorReset
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func stateColor(state string) string {
	switch state {
	case "UP":
		return colorGreen
	case "DOWN":
		return colorRed
	default:
		return ""
	}
}

func tlsColor(status string) string {
	switch status {
	case "OK":
		return colorGreen
	case "WARNING":
		return colorYellow
	case "EXPIRED", "ERROR":
		return colorRed
	default:
		return ""
	}
}
