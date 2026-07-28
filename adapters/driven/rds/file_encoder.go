// Package rds implements the outbound.RDSEncoder port. The FileEncoder writes the
// RDS state to a file as line-oriented key=value pairs (PI/PS/PTY/RT/flags) — the
// interchange format hardware/software RDS tools (e.g. a MiniRDS or Pira encoder
// script) poll. It transports metadata only, never audio.
package rds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	drds "autodj/domain/rds"
	"autodj/ports/outbound"
)

// FileEncoder writes RDS state to a file path. Each Push atomically overwrites the
// file with the current state so a poller always reads a whole, consistent snapshot.
type FileEncoder struct {
	path string
}

// NewFileEncoder builds an encoder writing to path. The parent directory is
// created lazily on first Push.
func NewFileEncoder(path string) *FileEncoder {
	return &FileEncoder{path: path}
}

// Push renders the state and atomically replaces the target file. Write goes to a
// temp file in the same directory then renames, so a concurrent reader never sees
// a half-written state.
func (e *FileEncoder) Push(_ context.Context, s drds.State) error {
	if err := os.MkdirAll(filepath.Dir(e.path), 0o755); err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(render(s)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, e.path)
}

// render serializes the state to the key=value line format.
func render(s drds.State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PI=%04X\n", s.PI)
	fmt.Fprintf(&b, "PS=%s\n", s.PS)
	fmt.Fprintf(&b, "PTY=%d\n", s.PTY)
	fmt.Fprintf(&b, "RT=%s\n", s.RT)
	fmt.Fprintf(&b, "TP=%s\n", boolFlag(s.TP))
	fmt.Fprintf(&b, "TA=%s\n", boolFlag(s.TA))
	fmt.Fprintf(&b, "MS=%s\n", boolFlag(s.MS))
	return b.String()
}

func boolFlag(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// compile-time check.
var _ outbound.RDSEncoder = (*FileEncoder)(nil)
