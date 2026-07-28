package content

import (
	"testing"

	"autodj/domain/library"
	"autodj/domain/mixing"
)

func TestOperationalCatalog(t *testing.T) {
	op := OperationalCatalog()
	if len(op.Categories) != 13 {
		t.Fatalf("want 13 categories, got %d", len(op.Categories))
	}
	if op.Categories[1].Key != "musicas" {
		t.Fatalf("bad category order: %+v", op.Categories[1])
	}
	if op.Categories[3].Key != "youtube" {
		t.Fatalf("bad category order: %+v", op.Categories[3])
	}
	if len(op.MusicSubtabs) != 4 {
		t.Fatalf("want 4 subtabs, got %d", len(op.MusicSubtabs))
	}
	if len(op.Folders) != 2 || op.Folders[0].Count != 42 {
		t.Fatalf("bad folders: %+v", op.Folders)
	}
	if len(op.Cartwall) != 20 {
		t.Fatalf("want 20 cart buttons, got %d", len(op.Cartwall))
	}
	if len(op.Markers) != 7 {
		t.Fatalf("want 7 markers, got %d", len(op.Markers))
	}
	if op.Mix.MusicaIn != 500 || op.Mix.NormalizeVol != 89 || !op.Mix.Normalize {
		t.Fatalf("bad mix defaults: %+v", op.Mix)
	}
}

// TestMarkerPaletteParity guards the two-engine contract at the seam: every
// palette key must be a real library.MarkerKind that .Valid() accepts, so each
// button on the editor produces a marker that survives PUT /markers.
func TestMarkerPaletteParity(t *testing.T) {
	for _, m := range OperationalCatalog().Markers {
		if !library.MarkerKind(m.Key).Valid() {
			t.Fatalf("palette key %q is not a valid MarkerKind", m.Key)
		}
		if m.Label == "" || m.Ico == "" || m.Color == "" {
			t.Fatalf("palette %q incomplete: %+v", m.Key, m)
		}
	}
}

func TestWebTabs(t *testing.T) {
	tabs := WebTabs()
	if len(tabs) != 8 {
		t.Fatalf("want 8 tabs, got %d", len(tabs))
	}
	// Sociais has 3 subtabs; others have none.
	var sociais *WebTab
	for i := range tabs {
		if tabs[i].Name == "Sociais" {
			sociais = &tabs[i]
		}
	}
	if sociais == nil || len(sociais.Subtabs) != 3 {
		t.Fatalf("Sociais should carry 3 subtabs: %+v", sociais)
	}
	if tabs[0].Framed {
		t.Fatalf("Notícias should not be framed")
	}
}

func TestProgramPreview(t *testing.T) {
	pg := ProgramPreview()
	if len(pg.Above) != 9 {
		t.Fatalf("want 9 above rows, got %d", len(pg.Above))
	}
	if len(pg.Below) != 10 {
		t.Fatalf("want 10 below rows, got %d", len(pg.Below))
	}
	if pg.Above[0].Kind != "music" || pg.Above[0].Label == "" {
		t.Fatalf("bad first above row: %+v", pg.Above[0])
	}
}

func TestGradeLoadPreview(t *testing.T) {
	gl := GradeLoadPreview()
	if len(gl.Hourly) != 24 {
		t.Fatalf("want 24 hourly entries, got %d", len(gl.Hourly))
	}
	for h, v := range gl.Hourly {
		if v < 0 {
			t.Fatalf("hour %d has negative load %d", h, v)
		}
	}
}

// TestMixDefaultParity guards that the UI-seed default (content.MixConfig, the
// fallback the frontend renders before /mix loads) never drifts from the domain
// factory default (mixing.Default(), the value the persisted repo returns when
// empty). Both share the same JSON shape; a mismatch would show one set of
// numbers in the modal and persist another.
// TestEmptyGradeLoad guards the clean-install histogram: 24 zeroed hours (no
// mock occupancy) before any grid is programmed.
func TestEmptyGradeLoad(t *testing.T) {
	gl := EmptyGradeLoad()
	if len(gl.Hourly) != 24 {
		t.Fatalf("want 24 hourly entries, got %d", len(gl.Hourly))
	}
	for h, v := range gl.Hourly {
		if v != 0 {
			t.Fatalf("hour %d must be zero, got %d", h, v)
		}
	}
}

func TestMixDefaultParity(t *testing.T) {
	c := OperationalCatalog().Mix
	d := mixing.Default()
	if c.MusicaIn != d.MusicaIn || c.MusicaOut != d.MusicaOut ||
		c.VinhetaIn != d.VinhetaIn || c.VinhetaOut != d.VinhetaOut ||
		c.ComercialIn != d.ComercialIn || c.ComercialOut != d.ComercialOut ||
		c.OverlapVol != d.OverlapVol || c.TalkVol != d.TalkVol ||
		c.TrimMusicas != d.TrimMusicas || c.TrimVinhetas != d.TrimVinhetas ||
		c.Normalize != d.Normalize || c.NormalizeVol != d.NormalizeVol ||
		c.DetectVocal != d.DetectVocal ||
		c.AutoInstrumental != d.AutoInstrumental {
		t.Fatalf("mix default drift:\n content=%+v\n mixing =%+v", c, d)
	}
}
