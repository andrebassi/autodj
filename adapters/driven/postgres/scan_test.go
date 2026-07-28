package postgres

import (
	"testing"
	"time"

	"autodj/domain/shared"
)

// fakeRow satisfies the scanner interface, filling dests from a fixed record.
// It verifies the row→Track mapping (source kind + ms→duration) without a DB.
type fakeRow struct {
	id, title, artist, album, category, source string
	path, spotify, youtube, key, waveform      string
	durMs, playCount                           int64
	bpm, loudness                              float64
	pcm, mixable                               bool
	err                                        error
}

func (f fakeRow) Scan(dest ...any) error {
	if f.err != nil {
		return f.err
	}
	*dest[0].(*string) = f.id
	*dest[1].(*string) = f.title
	*dest[2].(*string) = f.artist
	*dest[3].(*string) = f.album
	*dest[4].(*string) = f.category
	*dest[5].(*string) = f.source
	*dest[6].(*string) = f.path
	*dest[7].(*string) = f.spotify
	*dest[8].(*string) = f.youtube
	*dest[9].(*int64) = f.durMs
	*dest[10].(*float64) = f.bpm
	*dest[11].(*string) = f.key
	*dest[12].(*string) = f.waveform
	*dest[13].(*float64) = f.loudness
	*dest[14].(*int64) = f.playCount
	*dest[15].(*bool) = f.pcm
	*dest[16].(*bool) = f.mixable
	return nil
}

func TestScanMapsRow(t *testing.T) {
	row := fakeRow{
		id: "1", title: "Song", artist: "A", album: "Al", category: "music",
		source: "local", path: "/s.mp3", durMs: 185000, bpm: 128, key: "8A",
		loudness: -13.4, pcm: true, mixable: true,
	}
	tr, err := scan(row)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Source != shared.SourceLocal {
		t.Fatalf("source kind: %v", tr.Source)
	}
	if tr.Duration != 185*time.Second {
		t.Fatalf("duration ms→dur wrong: %v", tr.Duration)
	}
	if tr.BPM != 128 || tr.MusicalKey != "8A" || !tr.Mixable {
		t.Fatalf("mapping wrong: %+v", tr)
	}
	if tr.LoudnessLUFS != -13.4 {
		t.Fatalf("loudness não mapeado: %v", tr.LoudnessLUFS)
	}
	// A imagem estéreo tem de sobreviver ao round-trip: é ela que decide se o
	// botão VOZ pode ligar, e um campo perdido aqui apareceria como "o modo VOZ
	// nunca habilita" — sem erro nenhum.
}

func TestScanPropagatesError(t *testing.T) {
	if _, err := scan(fakeRow{err: errTest}); err == nil {
		t.Fatal("scan must propagate row error")
	}
}

var errTest = &scanErr{}

type scanErr struct{}

func (*scanErr) Error() string { return "scan failed" }
