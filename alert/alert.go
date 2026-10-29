// Package alert sends rate-limited operator notifications (e.g. email) when
// weclaw hits errors that need human attention.
package alert

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

// Alert kinds.
const (
	KindAgentError     = "agent_error"
	KindAgentTimeout   = "agent_timeout"
	KindReplyFailed    = "reply_failed"
	KindSessionExpired = "session_expired"
	KindPollFailing    = "poll_failing"
	KindPanic          = "panic"
	KindFatal          = "fatal"
)

const (
	dedupeWindow = 30 * time.Minute
	maxPerHour   = 20
	queueSize    = 64
)

// Alert describes one error event.
type Alert struct {
	Kind    string
	Summary string // short one-line description, used in the subject
	Account string // WeChat bot account ID
	UserID  string
	Agent   string
	Err     string
	Time    time.Time
}

// Key identifies identical alerts for deduplication.
func (a Alert) Key() string {
	return strings.Join([]string{a.Kind, a.Account, a.Agent, truncate(a.Err, 200)}, "|")
}

// Sender delivers a formatted alert.
type Sender interface {
	Send(subject, body string) error
}

// Notifier rate-limits alerts and delivers them asynchronously via a Sender.
type Notifier struct {
	sender  Sender
	limiter *limiter
	queue   chan Alert
	tail    *LogTail
	host    string
}

// NewNotifier creates a notifier and starts its delivery goroutine.
// tail may be nil.
func NewNotifier(sender Sender, tail *LogTail) *Notifier {
	host, _ := os.Hostname()
	n := &Notifier{
		sender:  sender,
		limiter: newLimiter(dedupeWindow, maxPerHour),
		queue:   make(chan Alert, queueSize),
		tail:    tail,
		host:    host,
	}
	go n.loop()
	return n
}

// Notify queues an alert without blocking. Rate-limited or overflowing alerts are dropped.
func (n *Notifier) Notify(a Alert) {
	if a.Time.IsZero() {
		a.Time = time.Now()
	}
	if !n.limiter.allow(a.Key(), a.Time) {
		return
	}
	select {
	case n.queue <- a:
	default:
		log.Printf("[alert] queue full, dropping %s alert", a.Kind)
	}
}

// sendNow delivers an alert synchronously (bounded by timeout), bypassing the queue.
// Used right before the process exits.
func (n *Notifier) sendNow(a Alert, timeout time.Duration) {
	if a.Time.IsZero() {
		a.Time = time.Now()
	}
	if !n.limiter.allow(a.Key(), a.Time) {
		return
	}
	done := make(chan struct{})
	go func() {
		n.deliver(a)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("[alert] timed out sending %s alert", a.Kind)
	}
}

func (n *Notifier) loop() {
	for a := range n.queue {
		n.deliver(a)
	}
}

func (n *Notifier) deliver(a Alert) {
	var logLines []string
	if n.tail != nil {
		logLines = n.tail.Lines()
	}
	subject, body := Format(a, n.host, logLines)
	if err := n.sender.Send(subject, body); err != nil {
		log.Printf("[alert] failed to send %s alert: %v", a.Kind, err)
		return
	}
	log.Printf("[alert] sent %s alert: %s", a.Kind, subject)
}

// Format renders the email subject and body for an alert.
func Format(a Alert, host string, logLines []string) (subject, body string) {
	summary := a.Summary
	if summary == "" {
		summary = a.Err
	}
	summary = strings.Join(strings.Fields(summary), " ") // single line
	subject = fmt.Sprintf("[weclaw] %s: %s", a.Kind, truncate(summary, 100))

	var b strings.Builder
	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "%-9s %s\n", name+":", value)
		}
	}
	field("Time", a.Time.Local().Format("2006-01-02 15:04:05 MST"))
	field("Kind", a.Kind)
	field("Host", host)
	field("Account", a.Account)
	field("User", a.UserID)
	field("Agent", a.Agent)
	if a.Err != "" {
		fmt.Fprintf(&b, "\nError:\n%s\n", a.Err)
	}
	if len(logLines) > 0 {
		fmt.Fprintf(&b, "\nRecent log:\n%s\n", strings.Join(logLines, "\n"))
	}
	return subject, b.String()
}

// --- package-level default notifier ---

var std atomic.Pointer[Notifier]

// SetDefault installs the process-wide notifier used by Notify. Passing nil disables alerts.
func SetDefault(n *Notifier) { std.Store(n) }

// Notify sends an alert via the default notifier. It is a no-op if alerts are not configured.
func Notify(a Alert) {
	if n := std.Load(); n != nil {
		n.Notify(a)
	}
}

// NotifyFatal synchronously sends a fatal-exit alert via the default notifier.
func NotifyFatal(err error) {
	if n := std.Load(); n != nil && err != nil {
		n.sendNow(Alert{Kind: KindFatal, Err: err.Error()}, 20*time.Second)
	}
}

// CrashOnPanic must be deferred directly. On panic it logs the stack, sends a
// panic alert synchronously, and exits with status 2 (like an unrecovered panic).
func CrashOnPanic() {
	r := recover()
	if r == nil {
		return
	}
	stack := debug.Stack()
	log.Printf("panic: %v\n%s", r, stack)
	if n := std.Load(); n != nil {
		n.sendNow(Alert{
			Kind:    KindPanic,
			Summary: fmt.Sprint(r),
			Err:     fmt.Sprintf("%v\n\n%s", r, stack),
		}, 20*time.Second)
	}
	os.Exit(2)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
