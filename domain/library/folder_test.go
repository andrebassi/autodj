package library

import (
	"errors"
	"strings"
	"testing"
	"time"

	"autodj/domain/shared"
)

func TestNormalizeFolder(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    string
		wantErr error
	}{
		{"rock", "rock", nil},
		{"  anos   90  ", "anos 90", nil}, // colapsa espaços do meio e das pontas
		{"Anos 90", "Anos 90", nil},       // NÃO muda a caixa: é como o operador escreveu
		{"", "", ErrFolderEmpty},
		{"   ", "", ErrFolderEmpty},
		{strings.Repeat("a", MaxFolderName), strings.Repeat("a", MaxFolderName), nil},
		{strings.Repeat("a", MaxFolderName+1), "", ErrFolderTooLong},
		// acento conta como UM caractere (runas, não bytes)
		{strings.Repeat("é", MaxFolderName), strings.Repeat("é", MaxFolderName), nil},
	} {
		got, err := NormalizeFolder(tc.in)
		if !errors.Is(err, tc.wantErr) {
			t.Fatalf("NormalizeFolder(%q) err = %v, want %v", tc.in, err, tc.wantErr)
		}
		if got != tc.want {
			t.Fatalf("NormalizeFolder(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Sem isto o operador criaria "Rock" e "rock" como pastas diferentes sem perceber.
func TestSameFolder(t *testing.T) {
	iguais := [][2]string{
		{"rock", "Rock"}, {"anos 90", "  ANOS   90 "}, {"Rock", "rock"},
	}
	for _, p := range iguais {
		if !SameFolder(p[0], p[1]) {
			t.Fatalf("SameFolder(%q,%q) = false, queria true", p[0], p[1])
		}
	}
	if SameFolder("rock", "rap") {
		t.Fatal("pastas diferentes viraram iguais")
	}
}

func TestNormalizeFolders(t *testing.T) {
	got, err := NormalizeFolders([]string{" rock ", "Rock", "", "anos 90", "   "})
	if err != nil {
		t.Fatal(err)
	}
	// "Rock" some (duplicata de "rock", 1ª grafia vence); vazios descartados; ordenado
	if len(got) != 2 || got[0] != "anos 90" || got[1] != "rock" {
		t.Fatalf("got = %v, queria [anos 90 rock]", got)
	}
}

func TestNormalizeFolders_limites(t *testing.T) {
	muitas := make([]string, MaxFoldersPerTrack+1)
	for i := range muitas {
		muitas[i] = string(rune('a'+i)) + "-pasta"
	}
	if _, err := NormalizeFolders(muitas); !errors.Is(err, ErrTooManyFolders) {
		t.Fatalf("err = %v, want ErrFoldersDemais", err)
	}
	if _, err := NormalizeFolders([]string{strings.Repeat("x", MaxFolderName+1)}); !errors.Is(err, ErrFolderTooLong) {
		t.Fatal("nome longo devia estourar")
	}
	if got, err := NormalizeFolders(nil); err != nil || len(got) != 0 {
		t.Fatalf("nil = %v/%v, queria lista vazia sem erro", got, err)
	}
}

// A pasta de streaming é DERIVADA da fonte: nunca desalinha e não migra nada.
func TestSourceFolderETrackFolders(t *testing.T) {
	if SourceFolder(shared.SourceSpotify) != FolderSpotify {
		t.Fatal("spotify")
	}
	if SourceFolder(shared.SourceYouTube) != FolderYouTube {
		t.Fatal("youtube")
	}
	if SourceFolder(shared.SourceLocal) != "" {
		t.Fatal("local não tem pasta automática")
	}

	// faixa de streaming ignora as pastas do operador (pasta única por serviço)
	sp := Track{Source: shared.SourceSpotify, Folders: []string{"rock"}}
	if f := TrackFolders(sp); len(f) != 1 || f[0] != FolderSpotify {
		t.Fatalf("TrackFolders(spotify) = %v, queria [Spotify]", f)
	}
	loc := Track{Source: shared.SourceLocal, Folders: []string{"rock", "anos 90"}}
	if f := TrackFolders(loc); len(f) != 2 {
		t.Fatalf("TrackFolders(local) = %v", f)
	}
}

// Validate é o funil: nenhum caminho de escrita pode gravar duplicata ou lixo.
func TestValidate_NormalizesFolders(t *testing.T) {
	tr := Track{
		ID: "x", Title: "T", Source: shared.SourceLocal, Path: "/a.mp3", Duration: time.Second,
		Folders: []string{" rock ", "ROCK", "anos 90"},
	}
	if err := tr.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(tr.Folders) != 2 {
		t.Fatalf("Folders = %v, queria duplicata removida", tr.Folders)
	}

	ruim := Track{ID: "y", Title: "T", Source: shared.SourceLocal, Path: "/b.mp3",
		Duration: time.Second,
		Folders:  []string{strings.Repeat("z", MaxFolderName+1)}}
	if err := ruim.Validate(); !errors.Is(err, ErrFolderTooLong) {
		t.Fatalf("err = %v, want ErrFolderLonga", err)
	}
}
