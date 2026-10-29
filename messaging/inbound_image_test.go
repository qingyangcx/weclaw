package messaging

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fastclaw-ai/weclaw/agent"
	"github.com/fastclaw-ai/weclaw/ilink"
)

func TestSaveInboundImages_SaveDir(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(png)
	}))
	defer srv.Close()

	h := newTestHandler()
	h.SetSaveDir(t.TempDir())

	atts, err := h.saveInboundImages(context.Background(), ilink.WeixinMessage{}, []*ilink.ImageItem{{URL: srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 || atts[0].MIMEType != "image/png" || !strings.HasSuffix(atts[0].Path, ".png") {
		t.Fatalf("unexpected attachments: %+v", atts)
	}
	if !strings.HasPrefix(atts[0].Path, h.saveDir) {
		t.Fatalf("image not saved in save_dir: %s", atts[0].Path)
	}
	if data, err := os.ReadFile(atts[0].Path); err != nil || string(data) != string(png) {
		t.Fatalf("saved data mismatch: %v", err)
	}
	if _, err := os.Stat(atts[0].Path + ".sidecar.md"); err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}
}

type textOnlyAgent struct{ got string }

func (a *textOnlyAgent) Chat(_ context.Context, _ string, message string) (string, error) {
	a.got = message
	return "ok", nil
}
func (a *textOnlyAgent) ResetSession(context.Context, string) (string, error) { return "", nil }
func (a *textOnlyAgent) Info() agent.AgentInfo                                { return agent.AgentInfo{Name: "fake"} }
func (a *textOnlyAgent) SetCwd(string)                                        {}

type attachmentAgent struct {
	textOnlyAgent
	atts []agent.Attachment
}

func (a *attachmentAgent) ChatWithAttachments(_ context.Context, _ string, message string, atts []agent.Attachment) (string, error) {
	a.got, a.atts = message, atts
	return "ok", nil
}

func TestChatWithAgent_Attachments(t *testing.T) {
	h := newTestHandler()
	atts := []agent.Attachment{{Path: "/tmp/x.jpg", MIMEType: "image/jpeg"}}

	textAg := &textOnlyAgent{}
	if _, err := h.chatWithAgent(context.Background(), textAg, "bot", "u", "what is this", atts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(textAg.got, "what is this") || !strings.Contains(textAg.got, "/tmp/x.jpg") {
		t.Fatalf("text agent should get path in prompt, got %q", textAg.got)
	}

	attAg := &attachmentAgent{}
	if _, err := h.chatWithAgent(context.Background(), attAg, "bot", "u", "what is this", atts); err != nil {
		t.Fatal(err)
	}
	if attAg.got != "what is this" || len(attAg.atts) != 1 {
		t.Fatalf("attachment agent got %q %+v", attAg.got, attAg.atts)
	}
}
