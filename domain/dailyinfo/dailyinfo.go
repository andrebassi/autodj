// Package dailyinfo computes the station's "info do dia" — a deterministic set of
// calendar/astronomy facts derived purely from a date: the moon phase and its
// illumination, the tropical sun sign, the season (southern hemisphere), the
// weekday, the day of the year and the national holidays that fall on it.
//
// It is 100% pure: Compute reads no wall clock and no network — it takes the date
// as input, so the same date always yields the same Info. Weather is intentionally
// out of scope (home use); only calendar and astronomical facts are computed here.
package dailyinfo

import (
	"math"
	"time"
)

// Info is the computed daily fact sheet for a single date. All fields are derived
// from Date alone by Compute, so Info is a pure projection of the calendar.
type Info struct {
	Date      time.Time
	Weekday   string // pt-BR weekday name (domingo…sábado)
	MoonPhase string // pt-BR moon phase name (8 phases)
	MoonIllum int    // illuminated fraction, 0..100 %
	Zodiac    string // pt-BR tropical sun sign
	Season    string // pt-BR season (southern hemisphere)
	DayOfYear int    // 1..366
	Holidays  []string
}

// synodicMonth is the mean length of a lunar cycle in days.
const synodicMonth = 29.530588853

// refNewMoonJD is the Julian Date of a known new moon (2000-01-06 18:14 UTC), the
// reference epoch for the moon-age calculation.
const refNewMoonJD = 2451550.1

// moonPhases are the eight pt-BR phase names in cycle order, starting at new moon.
var moonPhases = []string{
	"Lua nova",
	"Crescente côncava",
	"Quarto crescente",
	"Crescente gibosa",
	"Lua cheia",
	"Minguante gibosa",
	"Quarto minguante",
	"Minguante côncava",
}

// weekdays maps time.Weekday (Sunday=0) to its pt-BR name.
var weekdays = []string{
	"domingo", "segunda-feira", "terça-feira", "quarta-feira",
	"quinta-feira", "sexta-feira", "sábado",
}

// zodiacBoundaries lists the tropical sun signs by the exclusive upper bound of
// each range. pickRange returns the first sign whose (month, day) bound is strictly
// after the date; dates past the last bound (Dec 22–31) fall through to the wrap
// value Capricórnio.
var zodiacBoundaries = []dateRange{
	{time.January, 20, "Capricórnio"}, // Jan 1–19
	{time.February, 19, "Aquário"},    // Jan 20 – Feb 18
	{time.March, 21, "Peixes"},        // Feb 19 – Mar 20
	{time.April, 20, "Áries"},         // Mar 21 – Apr 19
	{time.May, 21, "Touro"},           // Apr 20 – May 20
	{time.June, 21, "Gêmeos"},         // May 21 – Jun 20
	{time.July, 23, "Câncer"},         // Jun 21 – Jul 22
	{time.August, 23, "Leão"},         // Jul 23 – Aug 22
	{time.September, 23, "Virgem"},    // Aug 23 – Sep 22
	{time.October, 23, "Libra"},       // Sep 23 – Oct 22
	{time.November, 22, "Escorpião"},  // Oct 23 – Nov 21
	{time.December, 22, "Sagitário"},  // Nov 22 – Dec 21
}

// seasonBoundaries lists the southern-hemisphere seasons by the exclusive upper
// bound of each range (approximate fixed dates). Same first-match-after rule as
// the zodiac table; dates past the last bound (Dec 21–31) fall through to the wrap
// value Verão.
var seasonBoundaries = []dateRange{
	{time.March, 21, "Verão"},       // Jan 1 – Mar 20
	{time.June, 21, "Outono"},       // Mar 21 – Jun 20
	{time.September, 23, "Inverno"},  // Jun 21 – Sep 22
	{time.December, 21, "Primavera"}, // Sep 23 – Dec 20
}

// holidays maps "MM-DD" to the national holidays on that day (fixed-date only).
var holidays = map[string][]string{
	"01-01": {"Confraternização Universal"},
	"04-21": {"Tiradentes"},
	"05-01": {"Dia do Trabalho"},
	"09-07": {"Independência do Brasil"},
	"10-12": {"Nossa Senhora Aparecida"},
	"11-02": {"Finados"},
	"11-15": {"Proclamação da República"},
	"12-25": {"Natal"},
}

// Compute derives the daily fact sheet for date. It is pure and deterministic:
// the moon phase/illumination from the synodic cycle, the tropical sun sign and
// southern-hemisphere season from date ranges, the weekday and day-of-year, and
// any fixed national holidays. Date is normalised to its calendar day (the time
// component only affects the moon age, which is fine).
func Compute(date time.Time) Info {
	phase, illum := moon(date)
	return Info{
		Date:      date,
		Weekday:   weekdays[int(date.Weekday())],
		MoonPhase: phase,
		MoonIllum: illum,
		Zodiac:    pickRange(date, zodiacBoundaries, "Capricórnio"),
		Season:    pickRange(date, seasonBoundaries, "Verão"),
		DayOfYear: date.YearDay(),
		Holidays:  holidaysFor(date),
	}
}

type dateRange struct {
	m    time.Month
	d    int
	name string
}

// pickRange returns the name of the first range whose (month, day) upper bound is
// strictly after the date. Dates past every bound (the year-end tail) fall through
// to wrap — the sign/season that carries over into the new year.
func pickRange(date time.Time, ranges []dateRange, wrap string) string {
	m, d := date.Month(), date.Day()
	for _, r := range ranges {
		if m < r.m || (m == r.m && d < r.d) {
			return r.name
		}
	}
	return wrap
}

// holidaysFor returns the national holidays on date's month-day (nil if none).
func holidaysFor(date time.Time) []string {
	key := date.Format("01-02")
	if hs, ok := holidays[key]; ok {
		out := make([]string, len(hs))
		copy(out, hs)
		return out
	}
	return nil
}

// moon returns the pt-BR phase name and the illuminated percentage (0..100) for
// the instant. Age is the number of days since the reference new moon modulo the
// synodic month; the phase index buckets the cycle into eight, and illumination
// follows the cosine of the phase angle.
func moon(date time.Time) (string, int) {
	jd := float64(date.UTC().Unix())/86400.0 + 2440587.5
	age := math.Mod(jd-refNewMoonJD, synodicMonth)
	if age < 0 {
		age += synodicMonth
	}
	frac := age / synodicMonth
	idx := int(frac*8+0.5) % 8
	illum := int(math.Round((1 - math.Cos(2*math.Pi*frac)) / 2 * 100))
	return moonPhases[idx], illum
}
