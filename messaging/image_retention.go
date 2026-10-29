package messaging

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	// inboundImageRetention is how long saved inbound images are kept.
	inboundImageRetention = 90 * 24 * time.Hour
	// inboundImageCleanupInterval is how often expired images are purged.
	inboundImageCleanupInterval = 24 * time.Hour
)

// inboundImageNameRe matches files written by saveInboundImages:
// "<YYYYMMDD-HHMMSS>-<8 hex>.<ext>" and its optional ".sidecar.md".
var inboundImageNameRe = regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{8}\.(png|jpg|gif|webp|bmp)(\.sidecar\.md)?$`)

// StartImageRetention purges expired inbound images now and then every 24h until ctx is done.
func (h *Handler) StartImageRetention(ctx context.Context) {
	dir := h.inboundImageDir()
	go func() {
		ticker := time.NewTicker(inboundImageCleanupInterval)
		defer ticker.Stop()
		for {
			if n, err := cleanupInboundImages(dir, inboundImageRetention, time.Now()); err != nil {
				log.Printf("[handler] image cleanup in %s failed: %v", dir, err)
			} else if n > 0 {
				log.Printf("[handler] removed %d expired image file(s) from %s", n, dir)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// cleanupInboundImages deletes weclaw-created image and sidecar files in dir
// (non-recursively) whose mtime is older than maxAge. It returns the number removed.
func cleanupInboundImages(dir string, maxAge time.Duration, now time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := now.Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !inboundImageNameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			log.Printf("[handler] failed to remove expired image %s: %v", e.Name(), err)
			continue
		}
		removed++
	}
	return removed, nil
}
