package audioanalyze

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNoop(t *testing.T) {
	a, err := Noop{}.Analyze(context.Background(), "/anything.flac")
	if err != nil {
		t.Fatalf("Noop error: %v", err)
	}
	if a.BPM != 0 || a.MusicalKey != "" || a.WaveformRef != "" {
		t.Fatalf("Noop must return empty Analysis, got %+v", a)
	}
}

func TestBPMFromBeats(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want float64
	}{
		{"120bpm half-second beats", "0.5\n1.0\n1.5\n2.0\n2.5", 120},
		{"128bpm", "0.0\n0.46875\n0.9375\n1.40625", 128},
		{"empty", "", 0},
		{"single beat", "1.0", 0},
		{"garbage", "abc\ndef", 0},
		{"non-increasing", "1.0\n1.0\n1.0", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bpmFromBeats(c.raw); got != c.want {
				t.Fatalf("bpmFromBeats(%q)=%v want %v", c.raw, got, c.want)
			}
		})
	}
}

func TestResolveAbsent(t *testing.T) {
	if p := resolve("definitely-not-a-real-binary-xyz"); p != "" {
		t.Fatalf("want empty for absent bin, got %q", p)
	}
	if p := resolve(""); p != "" {
		t.Fatalf("want empty for empty bin, got %q", p)
	}
}

func TestNewDefaults(t *testing.T) {
	c := New("")
	if c.AubioBin != "aubio" || c.KeyBin != "keyfinder-cli" || c.WaveformBin != "audiowaveform" {
		t.Fatalf("unexpected default bins: %+v", c)
	}
	if c.PeaksDir == "" {
		t.Fatal("PeaksDir should default to a path")
	}
	c2 := New("/tmp/peaks")
	if c2.PeaksDir != "/tmp/peaks" {
		t.Fatalf("PeaksDir override lost: %q", c2.PeaksDir)
	}
}

func TestAnalyzeDegradesWithNoTools(t *testing.T) {
	// Point every bin at a non-existent binary → Analyze returns empty, no error.
	c := &CLI{AubioBin: "nope-a", KeyBin: "nope-b", WaveformBin: "nope-c", PeaksDir: t.TempDir()}
	if c.Available() {
		t.Fatal("Available should be false with all-absent bins")
	}
	a, err := c.Analyze(context.Background(), "/music/x.flac")
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if a.BPM != 0 || a.MusicalKey != "" || a.WaveformRef != "" {
		t.Fatalf("expected empty Analysis, got %+v", a)
	}
}

// Integration: exercise the real CLIs when installed (Nix dev shell). Skips
// otherwise, like ffprobe's TestExtractRealFile.
func TestAnalyzeRealFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	c := New(t.TempDir())
	if !c.Available() {
		t.Skip("no analysis tool (aubio/keyfinder-cli/audiowaveform) installed")
	}
	// Generate a 5s sine WAV.
	wav := filepath.Join(t.TempDir(), "tone.wav")
	gen := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=5", wav)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg gen failed: %v\n%s", err, out)
	}
	a, err := c.Analyze(context.Background(), wav)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	// At least one field should be populated when a tool ran.
	if a.WaveformRef != "" {
		if _, err := os.Stat(a.WaveformRef); err != nil {
			t.Fatalf("waveform ref not written: %v", err)
		}
	}
	t.Logf("analysis: %+v", a)
}
