package agent

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessageWithAttachmentPaths(t *testing.T) {
	if got := MessageWithAttachmentPaths("hi", nil); got != "hi" {
		t.Fatalf("no attachments: got %q", got)
	}
	got := MessageWithAttachmentPaths("describe", []Attachment{{Path: "/tmp/a.png", MIMEType: "image/png"}})
	if !strings.HasPrefix(got, "describe") || !strings.Contains(got, "/tmp/a.png") {
		t.Fatalf("unexpected message: %q", got)
	}
}

func TestBuildPromptEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(path, []byte("pngdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	atts := []Attachment{{Path: path, MIMEType: "image/png"}}

	// Image-capable agent: text block + base64 image block, path not in text.
	entries, err := buildPromptEntries("describe", atts, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Type != "text" || entries[0].Text != "describe" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	img := entries[1]
	if img.Type != "image" || img.MIMEType != "image/png" || img.Data != base64.StdEncoding.EncodeToString([]byte("pngdata")) {
		t.Fatalf("unexpected image entry: %+v", img)
	}

	// Agent without image capability: path is included in the text only.
	entries, err = buildPromptEntries("describe", atts, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Text, path) {
		t.Fatalf("unexpected fallback entries: %+v", entries)
	}
}
