package alert

import (
	"sync"
	"time"
)

// limiter drops alerts whose key was seen within window, and caps total
// alerts to max per rolling hour.
type limiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	last   map[string]time.Time
	sent   []time.Time
}

func newLimiter(window time.Duration, max int) *limiter {
	return &limiter{window: window, max: max, last: make(map[string]time.Time)}
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	for k, t := range l.last {
		if now.Sub(t) >= l.window {
			delete(l.last, k)
		}
	}
	if _, dup := l.last[key]; dup {
		return false
	}

	i := 0
	for i < len(l.sent) && now.Sub(l.sent[i]) >= time.Hour {
		i++
	}
	l.sent = l.sent[i:]
	if len(l.sent) >= l.max {
		return false
	}

	l.last[key] = now
	l.sent = append(l.sent, now)
	return true
}
