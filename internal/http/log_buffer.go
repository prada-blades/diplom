package http

import (
	"strings"
	"sync"
)

type LogBuffer struct {
	mu       sync.RWMutex
	lines    []string
	pending  string
	maxLines int
}

func NewLogBuffer(maxLines int) *LogBuffer {
	if maxLines <= 0 {
		maxLines = 200
	}

	return &LogBuffer{maxLines: maxLines}
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	chunk := b.pending + string(p)
	parts := strings.Split(chunk, "\n")
	b.pending = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.lines = append(b.lines, line)
	}

	if extra := len(b.lines) - b.maxLines; extra > 0 {
		b.lines = append([]string(nil), b.lines[extra:]...)
	}

	return len(p), nil
}

func (b *LogBuffer) Entries() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := append([]string(nil), b.lines...)
	if strings.TrimSpace(b.pending) != "" {
		out = append(out, b.pending)
	}

	return out
}
