package main

type Report struct {
	System    SystemInfo
	Results   []Result
	Total     int
	UpCount   int
	DownCount int
	Reasons   map[string]int
}

func buildReport(system SystemInfo, results []Result) Report {
	total := len(results)
	upCount := 0

	for _, result := range results {
		if result.Up {
			upCount++
		}
	}

	return Report{
		System:    system,
		Results:   results,
		Total:     total,
		UpCount:   upCount,
		DownCount: total - upCount,
		Reasons:   countReasons(results),
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
