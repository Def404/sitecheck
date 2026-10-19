package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

var (
	ErrSitesDown   = errors.New("sites are down")
	errInterrupted = errors.New("interrupted by user")
)

// version, commit, buildDate — значения подставляются через ldflags при сборке.
// См. Makefile: -X main.version=... -X main.commit=... -X main.buildDate=...
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func printVersion(w *os.File) error {
	_, err := fmt.Fprintf(w, "sitecheck %s (commit %s, %s, %s/%s)\n",
		version, commit, buildDate, runtime.GOOS, runtime.GOARCH)
	return err
}

func main() {
	err := run(os.Args[1:])
	switch {
	case errors.Is(err, errInterrupted):
		os.Exit(130)
	case errors.Is(err, ErrSitesDown):
		os.Exit(1)
	case err != nil:
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
