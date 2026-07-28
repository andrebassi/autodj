// Package library (adapter) walks a directory tree for audio files.
package library

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
)

// audioExts are the extensions the scanner treats as playable audio.
var audioExts = map[string]bool{
	".mp3": true, ".flac": true, ".wav": true, ".ogg": true,
	".m4a": true, ".aac": true, ".opus": true, ".aiff": true,
}

// FolderScanner implements outbound.FolderScanner via filepath.WalkDir.
type FolderScanner struct{}

// New returns a FolderScanner.
func New() *FolderScanner { return &FolderScanner{} }

// Scan walks root and returns audio file paths (sorted by WalkDir order).
func (s *FolderScanner) Scan(ctx context.Context, root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		if audioExts[strings.ToLower(filepath.Ext(path))] {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}
