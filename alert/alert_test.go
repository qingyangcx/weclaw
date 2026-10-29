package alert

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLimiterDedupe(t *testing.T) {
	l := newLimiter(30*time.Minute, 20)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if !l.allow("a", t0) {
		t.Fatal("first alert should be allowed")
	}
	if l.allow("a", t0.Add(29*time.Minute)) {
		t.Fatal("duplicate within 30m should be suppressed")
	}
	if !l.allow("b", t0.Add(time.Minute)) {
		t.Fatal("different key should be allowed")
	}
	if !l.allow("a", t0.Add(30*time.Minute)) {
		t.Fatal("duplicate after 30m should be allowed")
	}
}

func TestLimiterHourlyCap(t *testing.T) {
	l := newLimiter(30*time.Minute, 20)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		if !l.allow(string(rune('a'+i)), t0.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("alert %d should be allowed", i)
		}
	}
	if l.allow("over", t0.Add(time.Minute)) {
		t.Fatal("21st alert within the hour should be dropped")
	}
	if !l.allow("over", t0.Add(time.Hour)) {
		t.Fatal("alert should be allowed once the oldest falls out of the hour window")
	}
}

func TestFormat(t *testing.T) {
	a := Alert{
		Kind:    KindAgentError,
		Summary: "claude: exit status 1\nstderr: boom",
		Account: "bot@im.bot",
		UserID:  "user@im.wechat",
		Agent:   "claude",
		Err:     "exit status 1",
		Time:    time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local),
	}
	subject, body := Format(a, "myhost", []string{"line one", "line two"})

	if want := "[weclaw] agent_error: claude: exit status 1 stderr: boom"; subject != want {
		t.Fatalf("subject = %q, want %q", subject, want)
	}
	for _, want := range []string{
		"Time:     2026-01-02 03:04:05",
		"Kind:     agent_error",
		"Host:     myhost",
		"Account:  bot@im.bot",
		"User:     user@im.wechat",
		"Agent:    claude",
		"Error:\nexit status 1",
		"Recent log:\nline one\nline two",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestFormatOmitsEmptyFieldsAndTruncatesSubject(t *testing.T) {
	subject, body := Format(Alert{Kind: KindFatal, Err: strings.Repeat("x", 300), Time: time.Now()}, "", nil)
	if !strings.HasPrefix(subject, "[weclaw] fatal: xxx") || !strings.HasSuffix(subject, "...") || len(subject) > 130 {
		t.Fatalf("unexpected subject %q", subject)
	}
	for _, absent := range []string{"User:", "Agent:", "Account:", "Host:", "Recent log:"} {
		if strings.Contains(body, absent) {
			t.Errorf("body should not contain %q:\n%s", absent, body)
		}
	}
}

type fakeSender struct {
	mu       sync.Mutex
	subjects []string
	bodies   []string
	sent     chan struct{}
}

func (f *fakeSender) Send(subject, body string) error {
	f.mu.Lock()
	f.subjects = append(f.subjects, subject)
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	f.sent <- struct{}{}
	return nil
}

func TestNotifierSendsAsyncAndDedupes(t *testing.T) {
	fs := &fakeSender{sent: make(chan struct{}, 10)}
	tail := NewLogTail(2)
	tail.Write([]byte("old\n"))
	tail.Write([]byte("ctx1\nctx2\n"))
	n := NewNotifier(fs, tail)

	a := Alert{Kind: KindReplyFailed, Account: "bot", UserID: "u1", Err: "send message: timeout"}
	n.Notify(a)
	n.Notify(a) // duplicate, suppressed

	select {
	case <-fs.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("alert was not sent")
	}
	select {
	case <-fs.sent:
		t.Fatal("duplicate alert should not be sent")
	case <-time.After(100 * time.Millisecond):
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.subjects[0] != "[weclaw] reply_failed: send message: timeout" {
		t.Fatalf("subject = %q", fs.subjects[0])
	}
	if !strings.Contains(fs.bodies[0], "Recent log:\nctx1\nctx2") || strings.Contains(fs.bodies[0], "old") {
		t.Fatalf("body should hold only the last 2 log lines:\n%s", fs.bodies[0])
	}
}

func TestNotifyWithoutDefaultIsNoop(t *testing.T) {
	SetDefault(nil)
	Notify(Alert{Kind: KindAgentError, Err: "x"}) // must not panic
}
