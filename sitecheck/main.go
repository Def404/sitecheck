package main

import (
	"errors"
	"fmt"
	"os"
)

var ErrSitesDown = errors.New("sites are down")

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
