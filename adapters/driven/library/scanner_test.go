package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanFindsAudioOnly(t *testing.T) {
	root := t.TempDir()
	must := func(rel string) {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("a.mp3")
	must("sub/b.FLAC") // case-insensitive ext
	must("c.txt")      // ignored
	must("d.jpg")      // ignored
	must("sub/e.wav")

	paths, err := New().Scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("want 3 audio files, got %d: %v", len(paths), paths)
	}
	for _, p := range paths {
		ext := filepath.Ext(p)
		if ext == ".txt" || ext == ".jpg" {
			t.Fatalf("non-audio leaked: %s", p)
		}
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := New().Scan(context.Background(), "/definitely/not/here/xyz"); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestScanContextCancel(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.mp3"), []byte("x"), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Scan(ctx, root); err == nil {
		t.Fatal("cancelled context should error")
	}
}
