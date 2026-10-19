package main

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// barWidth — число блоков в полосе прогресса.
const barWidth = 20

// progress — индикатор прогресса параллельной проверки: полоса
// «[██████░░░░] 60% (3/5)», перерисовываемая через \r на одной строке.
// Без ANSI-цветов. Активен только на TTY и при более чем одном URL.
type progress struct {
	w       io.Writer
	total   int
	enabled bool

	mu        sync.Mutex
	count     int64
	last      int
	lastWidth int
	stopped   bool
}

func newProgress(w io.Writer, total int, color bool) *progress {
	return &progress{
		w:       w,
		total:   total,
		enabled: color && total > 1,
	}
}

// tick — завершившая проверка обновляет счётчик и перерисовывает строку.
// Вызывается из goroutine; p.mu гарантирует, что только один tick
// пишет в p.w за раз.
func (p *progress) tick() {
	if !p.enabled {
		return
	}

	n := atomic.AddInt64(&p.count, 1)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped || int(n) == p.last {
		return
	}
	p.last = int(n)

	line := p.render(int(n))
	lineW := displayWidth(line)
	if lineW > p.lastWidth {
		p.lastWidth = lineW
	}

	out := make([]byte, 0, 1+p.lastWidth)
	out = append(out, '\r')
	out = append(out, line...)
	out = append(out, spaces(p.lastWidth-lineW)...)
	p.w.Write(out)
}

// finish — очищает строку прогресса и переходит на новую строку.
// Вызывается после wg.Wait() в run(), когда все tick() уже завершены.
func (p *progress) finish() {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopped = true

	width := p.lastWidth
	out := make([]byte, 0, 1+width+1)
	out = append(out, '\r')
	for i := 0; i < width; i++ {
		out = append(out, ' ')
	}
	out = append(out, '\n')
	p.w.Write(out)
}

// render — строка «[████░░░░] 40% (2/5)» без ANSI-кодов.
func (p *progress) render(n int) string {
	frac := float64(n) / float64(p.total)
	filled := int(frac*float64(barWidth) + 0.5)
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	pct := int(frac*100 + 0.5)
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}

	return fmt.Sprintf("[%s] %3d%% (%d/%d)", bar(barWidth, filled), pct, n, p.total)
}

// bar — строка из width блоков: filled заполнены, остальные пустые.
func bar(width, filled int) string {
	b := make([]rune, width)
	for i := range b {
		if i < filled {
			b[i] = '█'
		} else {
			b[i] = '░'
		}
	}
	return string(b)
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
