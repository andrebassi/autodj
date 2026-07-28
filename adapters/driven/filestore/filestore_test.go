package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }

func TestSaveAndRead(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	ctx := context.Background()
	path, err := s.Save(ctx, "song.mp3", strings.NewReader("audio"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != base {
		t.Fatalf("saved outside base: %q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "audio" {
		t.Fatalf("content mismatch: %q err=%v", b, err)
	}
}

func TestSaveSanitizesTraversal(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	path, err := s.Save(context.Background(), "../../etc/passwd", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	// Must land inside base, never escape it.
	if !strings.HasPrefix(path, base+string(os.PathSeparator)) {
		t.Fatalf("path escaped base: %q", path)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("path still has traversal: %q", path)
	}
}

func TestSaveEmptyNameFallback(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	path, err := s.Save(context.Background(), "...", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "upload" {
		t.Fatalf("empty-ish name should fall back to 'upload', got %q", filepath.Base(path))
	}
}

func TestRemoveWithinBase(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	ctx := context.Background()
	path, _ := s.Save(ctx, "a.mp3", strings.NewReader("x"))
	if err := s.Remove(ctx, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should be removed")
	}
	// Removing a missing file is not an error.
	if err := s.Remove(ctx, path); err != nil {
		t.Fatalf("removing missing file should be nil, got %v", err)
	}
}

func TestRemoveOutsideBaseIgnored(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	// A file outside base must never be deleted.
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(context.Background(), outside); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside file must be preserved")
	}
}

func TestNewDefaultBase(t *testing.T) {
	s := New("")
	if s.Base == "" {
		t.Fatal("default base must be set")
	}
}

func TestSaveMkdirError(t *testing.T) {
	// Base whose parent is a regular file → MkdirAll fails.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(filepath.Join(f, "sub"))
	if _, err := s.Save(context.Background(), "a.mp3", strings.NewReader("b")); err == nil {
		t.Fatal("MkdirAll should fail when parent is a file")
	}
}

func TestSaveCreateError(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	// Make dst path a directory so os.Create fails.
	if err := os.Mkdir(filepath.Join(base, "dir.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(context.Background(), "dir.mp3", strings.NewReader("b")); err == nil {
		t.Fatal("Create should fail when dst is a directory")
	}
}

func TestRemoveError(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	// Non-empty directory inside base → os.Remove fails (not IsNotExist).
	dir := filepath.Join(base, "sub")
	if err := os.MkdirAll(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(context.Background(), dir); err == nil {
		t.Fatal("removing non-empty dir should error")
	}
}

func TestSaveCopyError(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Save(context.Background(), "a.mp3", errReader{}); err == nil {
		t.Fatal("Copy should propagate reader error")
	}
}

// --- isolamento por estação (0.2.16) ---

func TestNewForWritesUnderTenantDir(t *testing.T) {
	root := t.TempDir()
	s := NewFor(root, "abc123")
	path, err := s.Save(context.Background(), "song.mp3", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "abc123")
	if filepath.Dir(path) != want {
		t.Fatalf("saved outside tenant dir: got %q want dir %q", path, want)
	}
}

func TestNewForSanitizesTenant(t *testing.T) {
	root := t.TempDir()
	// O tenant vira componente de path: separador e ".." têm de sumir.
	s := NewFor(root, "../../Evil Tenant/")
	path, err := s.Save(context.Background(), "a.mp3", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		t.Fatalf("tenant escaped root: %q", path)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("tenant still has traversal: %q", path)
	}
	if filepath.Dir(path) != filepath.Join(root, "eviltenant") {
		t.Fatalf("unexpected tenant dir: %q", filepath.Dir(path))
	}
}

func TestNewForEmptyTenantIsLegacyRoot(t *testing.T) {
	root := t.TempDir()
	// Tenant vazio (ou que colapse a vazio) = layout legado: grava na raiz.
	for _, tenant := range []string{"", "   ", "///"} {
		s := NewFor(root, tenant)
		if s.Base != root || s.Root != root {
			t.Fatalf("tenant %q: want legacy root layout, got Base=%q Root=%q", tenant, s.Base, s.Root)
		}
	}
}

// O ponto do isolamento: uma estação NÃO pode apagar o arquivo de outra, mesmo
// recebendo o path absoluto dele.
func TestRemoveOtherTenantIgnored(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	a := NewFor(root, "aaa")
	b := NewFor(root, "bbb")

	pathB, err := b.Save(ctx, "intro.mp3", strings.NewReader("b-audio"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(ctx, pathB); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Fatal("outro tenant não pode apagar o arquivo: deveria seguir existindo")
	}
	// E o dono apaga normalmente.
	if err := b.Remove(ctx, pathB); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pathB); !os.IsNotExist(err) {
		t.Fatal("o dono deveria conseguir remover")
	}
}

// Mesmo nome em estações diferentes = arquivos diferentes (era a colisão que
// sobrescrevia o áudio e batia no mesmo id `sha1(path)`).
func TestSameFilenameDifferentTenantsDoNotCollide(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	pa, err := NewFor(root, "aaa").Save(ctx, "intro.mp3", strings.NewReader("A"))
	if err != nil {
		t.Fatal(err)
	}
	pb, err := NewFor(root, "bbb").Save(ctx, "intro.mp3", strings.NewReader("B"))
	if err != nil {
		t.Fatal(err)
	}
	if pa == pb {
		t.Fatalf("paths colidiram: %q", pa)
	}
	ba, _ := os.ReadFile(pa)
	bb, _ := os.ReadFile(pb)
	if string(ba) != "A" || string(bb) != "B" {
		t.Fatalf("conteúdo sobrescrito: a=%q b=%q", ba, bb)
	}
}

// Acervo legado: arquivo solto na RAIZ (pré-isolamento) ainda é removível pelo
// tenant, senão apagar faixa antiga deixaria órfão pra sempre.
func TestRemoveLegacyRootFileAllowed(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "old.mp3")
	if err := os.WriteFile(legacy, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewFor(root, "aaa").Remove(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("arquivo legado da raiz deveria ser removível")
	}
}

func TestRemoveOutsideRootIgnoredWithTenant(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewFor(root, "aaa").Remove(context.Background(), outside); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("arquivo fora da raiz tem de ser preservado")
	}
}

func TestRemoveRootItselfIgnored(t *testing.T) {
	root := t.TempDir()
	if err := NewFor(root, "aaa").Remove(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("a própria raiz nunca pode ser removida")
	}
}

func TestNewForDefaultBase(t *testing.T) {
	s := NewFor("", "aaa")
	if s.Root == "" || s.Base == "" {
		t.Fatal("default root must be set")
	}
	if filepath.Base(s.Base) != "aaa" {
		t.Fatalf("tenant dir missing: %q", s.Base)
	}
}
