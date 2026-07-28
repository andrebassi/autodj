package ffprobe

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTagLookupCaseInsensitive(t *testing.T) {
	tags := map[string]string{"TITLE": "Song", "Artist": "Band"}
	if tag(tags, "title") != "Song" {
		t.Fatal("case-insensitive title lookup")
	}
	if tag(tags, "ARTIST") != "Band" {
		t.Fatal("case-insensitive artist lookup")
	}
	if tag(tags, "album") != "" {
		t.Fatal("missing tag returns empty")
	}
}

func TestEqualFold(t *testing.T) {
	if !equalFold("Title", "title") {
		t.Fatal("Title==title")
	}
	if equalFold("ab", "abc") {
		t.Fatal("length mismatch")
	}
	if equalFold("abc", "abd") {
		t.Fatal("last char differs")
	}
}

func TestExtractMissingFile(t *testing.T) {
	_, err := New().Extract(context.Background(), "/no/such/file.mp3")
	if err == nil {
		t.Fatal("missing file must error")
	}
}

// TestExtractRealFile generates a tagged audio file with ffmpeg and probes it.
func TestExtractRealFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "gen.mp3")
	// 1s sine, tagged.
	cmd := exec.Command("ffmpeg", "-nostdin", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-metadata", "title=Probe Test", "-metadata", "artist=AutoDJ",
		"-metadata", "album=Skeleton", "-y", out)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg gen failed: %v", err)
	}
	md, err := New().Extract(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if md.Title != "Probe Test" || md.Artist != "AutoDJ" || md.Album != "Skeleton" {
		t.Fatalf("tags wrong: %+v", md)
	}
	if md.Duration <= 0 {
		t.Fatalf("duration should be positive, got %v", md.Duration)
	}
}
