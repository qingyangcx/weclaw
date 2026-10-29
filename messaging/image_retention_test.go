package messaging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupInboundImages(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-91 * 24 * time.Hour)

	write := func(name string, mtime time.Time) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	// Old weclaw image + sidecar: should be removed.
	write("20260101-120000-abcdef01.jpg", old)
	write("20260101-120000-abcdef01.jpg.sidecar.md", old)
	write("20260101-120000-12345678.png", old)
	// Recent weclaw image + sidecar: kept.
	write("20261020-120000-deadbeef.webp", now.Add(-24*time.Hour))
	write("20261020-120000-deadbeef.webp.sidecar.md", now.Add(-24*time.Hour))
	// Old unrelated files: kept.
	write("notes.md", old)
	write("My Article.md.sidecar.md", old)
	write("photo.jpg", old)
	// Old matching file inside a subdirectory: kept (non-recursive).
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join("sub", "20260101-120000-abcdef02.jpg"), old)

	n, err := cleanupInboundImages(dir, 90*24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("removed %d files, want 3", n)
	}

	gone := []string{"20260101-120000-abcdef01.jpg", "20260101-120000-abcdef01.jpg.sidecar.md", "20260101-120000-12345678.png"}
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", name)
		}
	}
	kept := []string{"20261020-120000-deadbeef.webp", "20261020-120000-deadbeef.webp.sidecar.md", "notes.md", "My Article.md.sidecar.md", "photo.jpg", filepath.Join("sub", "20260101-120000-abcdef02.jpg")}
	for _, name := range kept {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should have been kept: %v", name, err)
		}
	}
}

func TestCleanupInboundImagesMissingDir(t *testing.T) {
	n, err := cleanupInboundImages(filepath.Join(t.TempDir(), "missing"), time.Hour, time.Now())
	if err != nil || n != 0 {
		t.Fatalf("got (%d, %v), want (0, nil)", n, err)
	}
}
