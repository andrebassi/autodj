package rds

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	drds "autodj/domain/rds"
)

func TestFileEncoderPush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "rds.txt") // sub/ must be created lazily
	enc := NewFileEncoder(path)

	st := drds.State{PI: 0x1001, PS: "AUTODJ", PTY: 10, RT: "♪ Os Ventania - Chuva de Neon", MS: true}
	if err := enc.Push(context.Background(), st); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	out := string(raw)
	for _, want := range []string{
		"PI=1001\n",
		"PS=AUTODJ\n",
		"PTY=10\n",
		"RT=♪ Os Ventania - Chuva de Neon\n",
		"TP=0\n",
		"TA=0\n",
		"MS=1\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q; got:\n%s", want, out)
		}
	}
	// no temp file left behind after atomic rename.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file should be renamed away")
	}
}

func TestFileEncoderOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rds.txt")
	enc := NewFileEncoder(path)
	ctx := context.Background()

	if err := enc.Push(ctx, drds.State{PI: 1, PS: "FIRST", RT: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := enc.Push(ctx, drds.State{PI: 2, PS: "SECOND", RT: "two", TA: true}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	out := string(raw)
	if strings.Contains(out, "FIRST") || strings.Contains(out, "one") {
		t.Fatalf("second push should overwrite; got:\n%s", out)
	}
	if !strings.Contains(out, "PS=SECOND\n") || !strings.Contains(out, "TA=1\n") {
		t.Fatalf("second state not written; got:\n%s", out)
	}
}

func TestFileEncoderBadPath(t *testing.T) {
	// path whose parent is a file, not a dir → MkdirAll fails.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	enc := NewFileEncoder(filepath.Join(f, "rds.txt"))
	if err := enc.Push(context.Background(), drds.State{PI: 1, PS: "X"}); err == nil {
		t.Fatal("expected error when parent is a file")
	}
}
