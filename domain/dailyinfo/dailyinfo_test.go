package dailyinfo

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 12, 0, 0, 0, time.UTC)
}

func TestComputeBasics(t *testing.T) {
	// 2026-07-24 is a Friday, day 205 of the year.
	info := Compute(d(2026, time.July, 24))
	if info.Weekday != "sexta-feira" {
		t.Fatalf("weekday: %q", info.Weekday)
	}
	if info.DayOfYear != 205 {
		t.Fatalf("day of year: %d", info.DayOfYear)
	}
	if !info.Date.Equal(d(2026, time.July, 24)) {
		t.Fatalf("date not echoed: %v", info.Date)
	}
	if info.MoonIllum < 0 || info.MoonIllum > 100 {
		t.Fatalf("illum out of range: %d", info.MoonIllum)
	}
	if info.Zodiac != "Leão" {
		t.Fatalf("zodiac: %q", info.Zodiac)
	}
	if info.Season != "Inverno" {
		t.Fatalf("season: %q", info.Season)
	}
}

func TestZodiacAllSigns(t *testing.T) {
	cases := []struct {
		date time.Time
		want string
	}{
		{d(2026, time.January, 10), "Capricórnio"},
		{d(2026, time.January, 25), "Aquário"},
		{d(2026, time.February, 25), "Peixes"},
		{d(2026, time.March, 25), "Áries"},
		{d(2026, time.April, 25), "Touro"},
		{d(2026, time.May, 25), "Gêmeos"},
		{d(2026, time.June, 25), "Câncer"},
		{d(2026, time.July, 25), "Leão"},
		{d(2026, time.August, 25), "Virgem"},
		{d(2026, time.September, 25), "Libra"},
		{d(2026, time.October, 25), "Escorpião"},
		{d(2026, time.November, 25), "Sagitário"},
		{d(2026, time.December, 25), "Capricórnio"}, // year-end wrap
	}
	for _, c := range cases {
		if got := Compute(c.date).Zodiac; got != c.want {
			t.Errorf("%s → zodiac %q, want %q", c.date.Format("01-02"), got, c.want)
		}
	}
}

func TestSeasonSouthernHemisphere(t *testing.T) {
	cases := []struct {
		date time.Time
		want string
	}{
		{d(2026, time.January, 15), "Verão"},     // early year
		{d(2026, time.April, 15), "Outono"},
		{d(2026, time.July, 15), "Inverno"},
		{d(2026, time.October, 15), "Primavera"},
		{d(2026, time.December, 25), "Verão"},    // year-end wrap
		{d(2026, time.March, 20), "Verão"},       // boundary: last summer day
		{d(2026, time.March, 21), "Outono"},      // boundary: first autumn day
	}
	for _, c := range cases {
		if got := Compute(c.date).Season; got != c.want {
			t.Errorf("%s → season %q, want %q", c.date.Format("01-02"), got, c.want)
		}
	}
}

func TestMoonCoversAllPhases(t *testing.T) {
	// Walk a full synodic cycle day by day and collect the distinct phase names
	// plus the illumination extremes — this exercises every phase bucket and the
	// 0%/100% rounding without hand-picking each date.
	seen := map[string]bool{}
	minIllum, maxIllum := 100, 0
	start := d(2024, time.January, 11) // near a new moon
	for i := 0; i < 30; i++ {
		info := Compute(start.AddDate(0, 0, i))
		seen[info.MoonPhase] = true
		if info.MoonIllum < minIllum {
			minIllum = info.MoonIllum
		}
		if info.MoonIllum > maxIllum {
			maxIllum = info.MoonIllum
		}
	}
	for _, name := range moonPhases {
		if !seen[name] {
			t.Errorf("phase %q never seen across a full cycle", name)
		}
	}
	if minIllum > 2 {
		t.Errorf("min illumination too high: %d (expected ~0 at new moon)", minIllum)
	}
	if maxIllum < 98 {
		t.Errorf("max illumination too low: %d (expected ~100 at full moon)", maxIllum)
	}
}

func TestMoonPre2000WrapsAge(t *testing.T) {
	// A date before the reference new moon makes (jd - ref) negative, exercising
	// the age-normalisation branch. Result must still be a valid phase + range.
	info := Compute(d(1999, time.January, 1))
	found := false
	for _, name := range moonPhases {
		if info.MoonPhase == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("pre-2000 phase invalid: %q", info.MoonPhase)
	}
	if info.MoonIllum < 0 || info.MoonIllum > 100 {
		t.Fatalf("pre-2000 illum out of range: %d", info.MoonIllum)
	}
}

func TestHolidays(t *testing.T) {
	if hs := Compute(d(2026, time.December, 25)).Holidays; len(hs) != 1 || hs[0] != "Natal" {
		t.Fatalf("Natal: %v", hs)
	}
	if hs := Compute(d(2026, time.April, 21)).Holidays; len(hs) != 1 || hs[0] != "Tiradentes" {
		t.Fatalf("Tiradentes: %v", hs)
	}
	// a plain day has no holiday
	if hs := Compute(d(2026, time.July, 4)).Holidays; hs != nil {
		t.Fatalf("expected no holiday, got %v", hs)
	}
	// returned slice is a copy: mutating it must not corrupt the shared map
	hs := Compute(d(2026, time.January, 1)).Holidays
	hs[0] = "MUTATED"
	if again := Compute(d(2026, time.January, 1)).Holidays; again[0] != "Confraternização Universal" {
		t.Fatalf("holiday map was mutated: %v", again)
	}
}

func TestWeekdaysAllNames(t *testing.T) {
	// 2026-07-19 is a Sunday; the seven following days cover every weekday name.
	want := []string{
		"domingo", "segunda-feira", "terça-feira", "quarta-feira",
		"quinta-feira", "sexta-feira", "sábado",
	}
	base := d(2026, time.July, 19)
	for i, w := range want {
		if got := Compute(base.AddDate(0, 0, i)).Weekday; got != w {
			t.Errorf("day+%d → %q, want %q", i, got, w)
		}
	}
}
