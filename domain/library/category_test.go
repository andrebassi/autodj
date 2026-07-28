package library

import (
	"testing"
	"time"

	"autodj/domain/shared"
)

func TestCategoriesFixedSet(t *testing.T) {
	cs := Categories()
	want := []Category{
		CategoryMusic, CategoryJingle, CategorySoundtrack, CategoryEffect,
		CategoryCommercial, CategoryPromo, CategorySponsorship, CategoryExtra,
	}
	if len(cs) != len(want) {
		t.Fatalf("want %d categories, got %d", len(want), len(cs))
	}
	for i, c := range want {
		if cs[i] != c {
			t.Fatalf("category[%d] = %q, want %q", i, cs[i], c)
		}
	}
	// Categories returns a copy — mutating it must not affect the canonical set.
	cs[0] = "hacked"
	if Categories()[0] != CategoryMusic {
		t.Fatal("Categories() must return a defensive copy")
	}
}

func TestValidCategory(t *testing.T) {
	for _, c := range Categories() {
		if !ValidCategory(c) {
			t.Fatalf("%q should be valid", c)
		}
	}
	for _, bad := range []Category{"", "bogus", "MUSIC", "musica"} {
		if ValidCategory(bad) {
			t.Fatalf("%q should be invalid", bad)
		}
	}
}

func TestDefaultCategoryIsMusic(t *testing.T) {
	if DefaultCategory != CategoryMusic {
		t.Fatalf("default = %q, want music", DefaultCategory)
	}
}

func TestValidateEmptyCategoryDefaultsToMusic(t *testing.T) {
	tr, err := NewTrack("id", shared.SourceLocal, "T", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.Path = "/x.mp3"
	tr.Category = ""
	if err := tr.Validate(); err != nil {
		t.Fatal(err)
	}
	if tr.Category != string(CategoryMusic) {
		t.Fatalf("empty category should default to music, got %q", tr.Category)
	}
}

func TestValidateRejectsBadCategory(t *testing.T) {
	tr, _ := NewTrack("id", shared.SourceLocal, "T", time.Minute)
	tr.Path = "/x.mp3"
	tr.Category = "bogus"
	if err := tr.Validate(); err != ErrBadCategory {
		t.Fatalf("bad category should return ErrBadCategory, got %v", err)
	}
}

func TestValidateAcceptsEveryCategory(t *testing.T) {
	for _, c := range Categories() {
		tr, _ := NewTrack("id", shared.SourceLocal, "T", time.Minute)
		tr.Path = "/x.mp3"
		tr.Category = string(c)
		if err := tr.Validate(); err != nil {
			t.Fatalf("category %q should validate: %v", c, err)
		}
	}
}
