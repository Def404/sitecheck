package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

var ErrSitesDown = errors.New("sites are down")

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
	if errors.Is(err, ErrSitesDown) {
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
