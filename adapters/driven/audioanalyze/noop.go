// Package audioanalyze implements outbound.AudioAnalyzer by shelling out to
// external CLIs (aubio for BPM, keyfinder-cli for Camelot key, audiowaveform
// for peaks). Out-of-process keeps the server cgo-free and dodges native-lib
// cross-compile pain (Wails). Every tool is optional: a missing binary yields a
// zero field, never an error — same graceful-degradation contract as the stub
// engine. Nix provides the tools in the dev shell (rule 23, local via Nix).
package audioanalyze

import (
	"context"

	"autodj/domain/library"
)

// Noop analyzes nothing and always returns an empty Analysis. It is the
// fallback when no analysis binary is present, so ingest still succeeds and
// tracks simply stay un-analyzed.
type Noop struct{}

// Analyze returns an empty Analysis.
func (Noop) Analyze(context.Context, string) (library.Analysis, error) {
	return library.Analysis{}, nil
}
