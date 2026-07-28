// Package ffprobe extracts media metadata by shelling out to ffprobe. Keeping
// this out-of-process keeps the server cgo-free and dodges native-lib
// cross-compile pain (Wails). ffprobe ships in the runtime image.
package ffprobe

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"time"

	"autodj/ports/outbound"
)

// Extractor implements outbound.MetadataExtractor.
type Extractor struct {
	// Bin is the ffprobe binary; defaults to "ffprobe" on PATH.
	Bin string
}

// New returns an Extractor using ffprobe from PATH.
func New() *Extractor { return &Extractor{Bin: "ffprobe"} }

type probeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

// Extract runs ffprobe -show_format on path and maps the result to Metadata.
func (e *Extractor) Extract(ctx context.Context, path string) (outbound.Metadata, error) {
	bin := e.Bin
	if bin == "" {
		bin = "ffprobe"
	}
	cmd := exec.CommandContext(ctx, bin,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		path,
	)
	raw, err := cmd.Output()
	if err != nil {
		return outbound.Metadata{}, err
	}
	var p probeOutput
	if err := json.Unmarshal(raw, &p); err != nil {
		return outbound.Metadata{}, err
	}
	md := outbound.Metadata{
		Title:  tag(p.Format.Tags, "title"),
		Artist: tag(p.Format.Tags, "artist"),
		Album:  tag(p.Format.Tags, "album"),
	}
	if secs, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil {
		md.Duration = time.Duration(secs * float64(time.Second))
	}
	if br, err := strconv.Atoi(p.Format.BitRate); err == nil {
		md.Bitrate = br
	}
	return md, nil
}

// tag does a case-insensitive lookup (ffprobe tag casing varies by container).
func tag(tags map[string]string, key string) string {
	for k, v := range tags {
		if equalFold(k, key) {
			return v
		}
	}
	return ""
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
