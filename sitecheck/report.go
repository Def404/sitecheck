package main

import "time"

type Report struct {
	System        SystemInfo
	Network       NetworkInfo
	Results       []Result
	Total         int
	UpCount       int
	DownCount     int
	CheckDuration time.Duration
	Reasons       map[string]int
}

func buildReport(system SystemInfo, network NetworkInfo, results []Result, checkDuration time.Duration) Report {
	total := len(results)
	upCount := 0
	skippedCount := 0

	for _, result := range results {
		switch {
		case result.Up:
			upCount++
		case result.Skipped:
			skippedCount++
		}
	}

	return Report{
		System:        system,
		Network:       network,
		Results:       results,
		Total:         total,
		UpCount:       upCount,
		DownCount:     total - upCount - skippedCount,
		CheckDuration: checkDuration,
		Reasons:       countReasons(results),
	}
}

func countReasons(results []Result) map[string]int {
	counts := make(map[string]int)
	for _, result := range results {
		if result.Reason == "" {
			continue
		}
		counts[result.Reason]++
	}
	return counts
}
