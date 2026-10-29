package alert

import (
	"strings"
	"sync"
)

// LogTail is an io.Writer that keeps the last N log lines in memory,
// so alerts can include recent context.
type LogTail struct {
	mu    sync.Mutex
	max   int
	lines []string
}

// NewLogTail creates a LogTail keeping up to max lines.
func NewLogTail(max int) *LogTail {
	return &LogTail{max: max}
}

func (t *LogTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		t.lines = append(t.lines, truncate(line, 500))
	}
	if over := len(t.lines) - t.max; over > 0 {
		t.lines = append(t.lines[:0:0], t.lines[over:]...)
	}
	return len(p), nil
}

// Lines returns a copy of the buffered lines, oldest first.
func (t *LogTail) Lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.lines...)
}
