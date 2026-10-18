package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type JSONResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Up         bool   `json:"up"`
	DurationMS int64  `json:"duration_ms"`
	Reason     string `json:"reason"`
	Attempts   int    `json:"attempts"`
}

type JSONReport struct {
	System          SystemInfo     `json:"system"`
	Network         NetworkInfo    `json:"network"`
	Total           int            `json:"total"`
	Up              int            `json:"up"`
	Down            int            `json:"down"`
	CheckDurationMS int64          `json:"check_duration_ms"`
	Reasons         map[string]int `json:"reasons"`
	Results         []JSONResult   `json:"results"`
}

func toJSONResult(result Result) JSONResult {
	return JSONResult{
		URL:        result.URL,
		StatusCode: result.StatusCode,
		Up:         result.Up,
		DurationMS: result.Duration.Milliseconds(),
		Reason:     result.Reason,
		Attempts:   result.Attempts,
	}
}

func toJSONReport(report Report) JSONReport {
	results := make([]JSONResult, 0, len(report.Results))
	for _, result := range report.Results {
		results = append(results, toJSONResult(result))
	}

	return JSONReport{
		System:          report.System,
		Network:         report.Network,
		Total:           report.Total,
		Up:              report.UpCount,
		Down:            report.DownCount,
		CheckDurationMS: report.CheckDuration.Milliseconds(),
		Reasons:         report.Reasons,
		Results:         results,
	}
}

func marshalResults(results []Result) ([]byte, error) {
	jsonResults := make([]JSONResult, 0, len(results))
	for _, result := range results {
		jsonResults = append(jsonResults, toJSONResult(result))
	}

	return json.MarshalIndent(jsonResults, "", "  ")
}

func marshalReport(report Report) ([]byte, error) {
	return json.MarshalIndent(toJSONReport(report), "", "    ")
}

func printJSON(result Result) error {
	data, err := json.MarshalIndent(toJSONResult(result), "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func printJSONResults(results []Result) error {
	data, err := marshalResults(results)
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func saveJSON(filename string, data []byte) error {
	return os.WriteFile(filename, data, 0o644)
}

func saveJSONResults(filename string, results []Result) error {
	data, err := marshalResults(results)
	if err != nil {
		return err
	}

	return saveJSON(filename, data)
}
