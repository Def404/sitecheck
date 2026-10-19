package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type Options struct {
	Path        string
	JSONOutput  bool
	JSONReport  bool
	OutputFile  string
	TextOutput  string
	Concurrency int
	Retries     int
	Version     bool
	Help        bool
}

const usage = `Использование: sitecheck [флаги] FILE

FILE — файл со списком URL (по одному на строку, # — комментарий).

Примеры:
  sitecheck sites.txt
  sitecheck --concurrency 8 --retries 1 sites.txt
  sitecheck --json-report --output report.json sites.txt
  sitecheck --json --text-output summary.txt sites.txt
  SITECHECK_TIMEOUT=10s sitecheck sites.txt
  sitecheck --version
`

func printHelp(w io.Writer) error {
	_, err := fmt.Fprint(w, usage)
	return err
}

func parseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("sitecheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	json := fs.Bool("json", false, "output results as JSON")
	jsonReport := fs.Bool("json-report", false, "output a JSON report")
	output := fs.String("output", "", "write JSON report to this file")
	textOutput := fs.String("text-output", "", "write the text summary to this file")
	concurrency := fs.Int("concurrency", 5, "maximum number of concurrent checks")
	retries := fs.Int("retries", 0, "number of retry attempts after the initial check fails")
	version := fs.Bool("version", false, "print version information and exit")
	help := fs.Bool("help", false, "print this help and exit")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if *version {
		return Options{Version: true}, nil
	}

	if *help {
		return Options{Help: true}, nil
	}

	if *concurrency <= 0 {
		return Options{}, fmt.Errorf("--concurrency must be a positive number, got %d", *concurrency)
	}

	if *retries < 0 {
		return Options{}, fmt.Errorf("--retries cannot be negative, got %d", *retries)
	}

	rest := fs.Args()
	if len(rest) != 1 {
		if len(rest) == 0 {
			return Options{}, errors.New("filename is required")
		}
		return Options{}, errors.New("too many arguments")
	}
	if rest[0] == "" {
		return Options{}, errors.New("filename is required")
	}
	if *output != "" && !*json && !*jsonReport {
		return Options{}, errors.New("--output requires --json or --json-report")
	}
	if *json && *jsonReport {
		return Options{}, errors.New("--json and --json-report cannot be used together")
	}
	if *textOutput != "" && (*json || *jsonReport) {
		return Options{}, errors.New("--text-output cannot be used with --json or --json-report")
	}

	return Options{
		Path:        rest[0],
		JSONOutput:  *json,
		JSONReport:  *jsonReport,
		OutputFile:  *output,
		TextOutput:  *textOutput,
		Concurrency: *concurrency,
		Retries:     *retries,
	}, nil
}

func getTimeout() (time.Duration, error) {
	value := os.Getenv("SITECHECK_TIMEOUT")

	if value == "" {
		return 5 * time.Second, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid SITECHECK_TIMEOUT %q: %w", value, err)
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("SITECHECK_TIMEOUT must be positive, got %s", timeout)
	}

	return timeout, nil
}
