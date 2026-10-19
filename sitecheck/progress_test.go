package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestProgressDisabledSingleURL(t *testing.T) {
	p := newProgress(io.Discard, 1, true)
	if p.enabled {
		t.Errorf("enabled = %v, want false (single URL)", p.enabled)
	}
	p.tick()
	p.finish()
}

func TestProgressDisabledNoColor(t *testing.T) {
	p := newProgress(io.Discard, 10, false)
	if p.enabled {
		t.Errorf("enabled = %v, want false (no color / non-TTY)", p.enabled)
	}
	p.tick()
	p.finish()
}

func TestProgressEnabled(t *testing.T) {
	p := newProgress(io.Discard, 3, true)
	if !p.enabled {
		t.Fatalf("enabled = %v, want true (multi URL + color)", p.enabled)
	}
	p.tick()
	p.tick()
	p.tick()
	p.finish()
}

func TestBar(t *testing.T) {
	tests := []struct {
		width  int
		filled int
		want   string
	}{
		{width: 5, filled: 0, want: "░░░░░"},
		{width: 5, filled: 2, want: "██░░░"},
		{width: 5, filled: 5, want: "█████"},
		{width: 5, filled: 9, want: "█████"}, // clamp up
		{width: 5, filled: -1, want: "░░░░░"}, // clamp down
	}
	for _, tt := range tests {
		if got := bar(tt.width, tt.filled); got != tt.want {
			t.Errorf("bar(%d, %d) = %q, want %q", tt.width, tt.filled, got, tt.want)
		}
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		total int
		n     int
		color bool
		want  string
	}{
		{total: 5, n: 0, color: false, want: "[░░░░░░░░░░░░░░░░░░░░]   0% (0/5)"},
		{total: 5, n: 1, color: false, want: "[████░░░░░░░░░░░░░░░░]  20% (1/5)"},
		{total: 5, n: 3, color: false, want: "[████████████░░░░░░░░]  60% (3/5)"},
		{total: 5, n: 5, color: false, want: "[████████████████████] 100% (5/5)"},
		{total: 3, n: 1, color: false, want: "[███████░░░░░░░░░░░░░]  33% (1/3)"},
		{total: 3, n: 2, color: false, want: "[█████████████░░░░░░░]  67% (2/3)"},
	}
	for _, tt := range tests {
		p := newProgress(io.Discard, tt.total, tt.color)
		if got := p.render(tt.n); got != tt.want {
			t.Errorf("render(n=%d, total=%d) = %q, want %q", tt.n, tt.total, got, tt.want)
		}
	}
}

func TestRenderColored(t *testing.T) {
	// Бар без ANSI-цветов: render() возвращает «[████...] 50% (2/4)».
	// Проверяем структуру строки для включённого прогресса (total > 1).
	p := newProgress(io.Discard, 4, true)
	got := p.render(2)
	if !strings.Contains(got, "50%") {
		t.Errorf("render(2, total=4) = %q, want it to contain 50%%", got)
	}
	if !strings.Contains(got, "2/4") {
		t.Errorf("render(2, total=4) = %q, want it to contain 2/4", got)
	}
}

func TestProgressTickWrites(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, 3, true)
	if !p.enabled {
		t.Fatalf("enabled = %v, want true (color && total>1)", p.enabled)
	}

	p.tick()
	first := buf.String()
	if !strings.Contains(first, "\r") {
		t.Errorf("tick() = %q, want it to contain \\r", first)
	}
	if !strings.Contains(first, "1/3") {
		t.Errorf("tick() = %q, want it to contain 1/3", first)
	}
	if !strings.Contains(first, "33%") {
		t.Errorf("tick() = %q, want it to contain 33%%", first)
	}

	buf.Reset()
	p.tick()
	p.tick()
	third := buf.String()
	if !strings.Contains(third, "3/3") {
		t.Errorf("third tick() = %q, want it to contain 3/3", third)
	}
	if !strings.Contains(third, "100%") {
		t.Errorf("third tick() = %q, want it to contain 100%%", third)
	}

	buf.Reset()
	p.finish()
	finished := buf.String()
	// Формат: \r + пробелы + \n (очистка строки + новая строка)
	if !strings.HasPrefix(finished, "\r") || !strings.HasSuffix(finished, "\n") {
		t.Errorf("finish() = %q, want \\r ... \\n", finished)
	}
	if strings.Contains(finished, "█") || strings.Contains(finished, "%") {
		t.Errorf("finish() = %q, want only spaces between \\r markers", finished)
	}

	buf.Reset()
	p.tick()
	if buf.Len() != 0 {
		t.Errorf("tick after finish wrote %q, want nothing", buf.String())
	}
}
