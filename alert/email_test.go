package alert

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("bot@example.com", []string{"a@example.com", "b@example.com"},
		"[weclaw] agent_error: 超时", "line1\nline2", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))

	for _, want := range []string{
		"From: bot@example.com\r\n",
		"To: a@example.com, b@example.com\r\n",
		"Subject: =?utf-8?q?",
		"Date: Fri, 02 Jan 2026 03:04:05 +0000\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\nline1\r\nline2",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}
