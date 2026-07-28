// Package filestore implements outbound.MediaStore on the local filesystem.
// Uploaded media lands under a base dir (default ~/.autodj/media). Filenames
// are sanitized to their base component so an upload can never escape the base
// dir (path traversal), and collisions overwrite in place — the caller derives
// a stable id from the returned path, so re-uploading the same name is an
// upsert.
//
// ISOLAMENTO POR ESTAÇÃO (0.2.16): cada tenant (fingerprint do browser) escreve
// no SEU subdiretório, `<root>/<tenant>/`. Antes o `media/` era global enquanto
// a biblioteca já era por sessão (`sessions/<fp>.db`), então duas estações que
// subissem `intro.mp3` sobrescreviam o arquivo uma da outra E colidiam no mesmo
// id (`sha1(path)`, e o path era idêntico). O tenant default ("default": curl,
// testes, primeiro paint) segue gravando na RAIZ — é o layout legado, e manter
// preserva as bibliotecas que já existem sem reescrever id nenhum.
package filestore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Store writes uploads under Base. Root é a raiz do media (a fronteira do
// Remove); com tenant, Base = Root/<tenant>.
type Store struct {
	Base string
	Root string
}

// tenantInvalid strips everything but [a-z0-9] — o mesmo alfabeto que o
// serverkit já aplica ao fingerprint. Defensivo: o nome do tenant vira
// componente de path, então nunca pode conter separador nem "..".
var tenantInvalid = regexp.MustCompile(`[^a-z0-9]+`)

// New returns a Store rooted at base. If base is empty it falls back to
// ~/.autodj/media (or ./autodj-media when the home dir is unknown).
func New(base string) *Store {
	base = rootOr(base)
	return &Store{Base: base, Root: base}
}

// NewFor returns a Store that WRITES into base/<tenant> but still accepts
// deletes anywhere sob a raiz que seja seu (ver Remove). tenant vazio (ou que
// colapse a vazio depois de sanitizado) devolve o layout legado — idêntico a
// New(base) —, que é o caso do tenant default.
func NewFor(base, tenant string) *Store {
	root := rootOr(base)
	t := tenantInvalid.ReplaceAllString(strings.ToLower(strings.TrimSpace(tenant)), "")
	if t == "" {
		return &Store{Base: root, Root: root}
	}
	return &Store{Base: filepath.Join(root, t), Root: root}
}

// rootOr resolves the media root, falling back to ~/.autodj/media.
func rootOr(base string) string {
	if base != "" {
		return base
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".autodj", "media")
	}
	return "autodj-media"
}

// sanitize reduces name to a safe base filename (no dirs, no traversal).
func sanitize(name string) string {
	name = filepath.Base(filepath.FromSlash(name))
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "..", "")
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "upload"
	}
	return name
}

// Save writes r under a sanitized name in Base and returns the stored path.
func (s *Store) Save(_ context.Context, filename string, r io.Reader) (string, error) {
	if err := os.MkdirAll(s.Base, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(s.Base, sanitize(filename))
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return dst, nil
}

// Remove deletes the file at path. A missing file is not an error.
//
// Duas fronteiras, não uma:
//   - dentro do PRÓPRIO Base (`<root>/<tenant>/…`) — o caso normal;
//   - arquivo solto na RAIZ do media (`<root>/x.mp3`, sem subdiretório) — o
//     acervo legado pré-isolamento, que as bibliotecas antigas ainda referenciam
//     pelo path absoluto guardado na linha do track. Sem essa exceção, apagar
//     uma faixa antiga deixaria o arquivo órfão pra sempre.
//
// Tudo o mais é ignorado: path fora da raiz (defensivo — nunca apagar arquivo
// arbitrário do disco) e, principalmente, o subdiretório de OUTRO tenant.
func (s *Store) Remove(_ context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return nil
	}
	base, err := filepath.Abs(s.Base)
	if err != nil {
		return nil
	}
	own := strings.HasPrefix(abs, base+string(os.PathSeparator))
	if !own {
		rel, err := filepath.Rel(root, abs)
		// legado só vale pra filho DIRETO da raiz: com separador no rel o
		// arquivo mora no subdiretório de alguém (outro tenant) — fora.
		if err != nil || rel == "." || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(os.PathSeparator)) ||
			strings.ContainsRune(rel, os.PathSeparator) {
			return nil // not ours (e.g. demo seed's fictional /demo path)
		}
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
