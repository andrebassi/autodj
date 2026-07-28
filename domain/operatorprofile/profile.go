// Package operatorprofile models each operator's saved player layout: theme,
// density and the ordered, show/hide list of panels. It is pure — no I/O, no
// deps beyond the standard library.
//
// A profile belongs to a user (identity.User.ID from the RBAC domain). The shell
// reads it at login and applies theme/density as CSS hints and the panel list as
// the visible, ordered set of panes. Ordering is always normalized to 0..n-1 so
// the frontend never has to reconcile gaps or duplicates.
package operatorprofile

import "errors"

// Errors returned when building or validating a profile.
var (
	// ErrBadTheme rejects a theme outside the known set.
	ErrBadTheme = errors.New("operatorprofile: unknown theme")
	// ErrBadDensity rejects a density outside the known set.
	ErrBadDensity = errors.New("operatorprofile: unknown density")
	// ErrDupPanel rejects a panel list with a repeated key.
	ErrDupPanel = errors.New("operatorprofile: duplicate panel key")
	// ErrUnknownPanel rejects a panel key not part of the known shell panels.
	ErrUnknownPanel = errors.New("operatorprofile: unknown panel key")
	// ErrBadScreens rejects a monitor count outside {1,2}.
	ErrBadScreens = errors.New("operatorprofile: screens must be 1 or 2")
	// ErrBadScreen rejects a panel monitor index outside {0,1}.
	ErrBadScreen = errors.New("operatorprofile: panel screen must be 0 or 1")
)

// Theme is the color scheme the shell applies as a CSS hint.
type Theme string

// The known themes.
const (
	// ThemeAuto follows the OS/browser preference.
	ThemeAuto Theme = "auto"
	// ThemeLight forces the light scheme.
	ThemeLight Theme = "claro"
	// ThemeDark forces the dark scheme.
	ThemeDark Theme = "escuro"
)

// Density is how tightly the shell packs its controls.
type Density string

// The known densities.
const (
	// DensityComfortable is the roomier default.
	DensityComfortable Density = "confortavel"
	// DensityCompact tightens spacing for small screens / power users.
	DensityCompact Density = "compacto"
)

// Themes and Densities list the known values, for validation and UI.
var (
	Themes    = []Theme{ThemeAuto, ThemeLight, ThemeDark}
	Densities = []Density{DensityComfortable, DensityCompact}
)

// PanelKey identifies one pane in the operator shell.
type PanelKey string

// The known shell panels — the panes an operator can reorder or hide.
const (
	PanelQueue    PanelKey = "fila"
	PanelLibrary  PanelKey = "biblioteca"
	PanelCartwall PanelKey = "cartwall"
	PanelSpotify  PanelKey = "spotify"
	PanelAIDJ     PanelKey = "aidj"
)

// Panels lists every known panel in its default display order. Default builds
// the starting layout from this list, so the order here is the shipped default.
var Panels = []PanelKey{PanelQueue, PanelLibrary, PanelCartwall, PanelSpotify, PanelAIDJ}

// knownPanel reports whether k is one of the shell's panels.
func knownPanel(k PanelKey) bool {
	for _, p := range Panels {
		if p == k {
			return true
		}
	}
	return false
}

// PanelPref is one operator preference for a panel: whether it shows, where it
// sits in the order, and which monitor it lives on (Story 9.2). Order is
// authoritative only after normalization; Screen is 0 (principal) unless the
// profile runs two monitors (Screens==2), in which case a panel may sit on 1.
type PanelPref struct {
	Key     PanelKey `json:"key"`
	Visible bool     `json:"visible"`
	Order   int      `json:"order"`
	Screen  int      `json:"screen"`
}

// Profile is one operator's saved layout. UserID ties it to the RBAC user; the
// panel list is kept normalized (contiguous 0..n-1 order, no duplicates). Touch
// and Screens are the Story 9.2 additions: touch mode enlarges hit targets, and
// Screens (1 or 2) is how many monitors the layout spreads across.
type Profile struct {
	UserID  string      `json:"userId"`
	Theme   Theme       `json:"theme"`
	Density Density     `json:"density"`
	Touch   bool        `json:"touch"`
	Screens int         `json:"screens"`
	Panels  []PanelPref `json:"panels"`
}

// validTheme / validDensity report membership in the known sets.
func validTheme(t Theme) bool {
	for _, k := range Themes {
		if k == t {
			return true
		}
	}
	return false
}

func validDensity(d Density) bool {
	for _, k := range Densities {
		if k == d {
			return true
		}
	}
	return false
}

// Default builds the shipped layout for a user: light+comfortable, every panel
// visible in the canonical order. Used when a user has no saved profile yet — o
// tema padrão (instalação limpa / pós-Limpar) é o CLARO.
func Default(userID string) Profile {
	prefs := make([]PanelPref, len(Panels))
	for i, k := range Panels {
		prefs[i] = PanelPref{Key: k, Visible: true, Order: i}
	}
	return Profile{
		UserID:  userID,
		Theme:   ThemeLight,
		Density: DensityComfortable,
		Touch:   false,
		Screens: 1,
		Panels:  prefs,
	}
}

// Validate checks the theme, density and panel list. It also normalizes the
// panel order in place (sorting by the given Order, reindexing to 0..n-1), so a
// valid Profile always leaves this method with a clean order. Unknown panel keys
// and duplicates are rejected rather than dropped, so a bad payload is loud.
func (p *Profile) Validate() error {
	if p.Screens == 0 {
		p.Screens = 1 // back-compat: profiles saved before Story 9.2 have no count
	}
	if p.Screens != 1 && p.Screens != 2 {
		return ErrBadScreens
	}
	if !validTheme(p.Theme) {
		return ErrBadTheme
	}
	if !validDensity(p.Density) {
		return ErrBadDensity
	}
	seen := make(map[PanelKey]bool, len(p.Panels))
	for _, pref := range p.Panels {
		if !knownPanel(pref.Key) {
			return ErrUnknownPanel
		}
		if seen[pref.Key] {
			return ErrDupPanel
		}
		if pref.Screen < 0 || pref.Screen > 1 {
			return ErrBadScreen
		}
		seen[pref.Key] = true
	}
	p.normalize()
	return nil
}

// normalize sorts panels by their Order (stable insertion — the list is tiny)
// and reindexes to a contiguous 0..n-1 range.
func (p *Profile) normalize() {
	// insertion sort by Order — n is at most len(Panels)
	for i := 1; i < len(p.Panels); i++ {
		for j := i; j > 0 && p.Panels[j-1].Order > p.Panels[j].Order; j-- {
			p.Panels[j-1], p.Panels[j] = p.Panels[j], p.Panels[j-1]
		}
	}
	for i := range p.Panels {
		p.Panels[i].Order = i
	}
	// single monitor ⇒ every panel lives on the principal screen (defensive:
	// a stale Screen=1 must not survive a switch back to one monitor).
	if p.Screens == 1 {
		for i := range p.Panels {
			p.Panels[i].Screen = 0
		}
	}
}

// Reorder moves the panel with key to position idx (clamped to the valid range)
// and reindexes the rest. Unknown keys are ignored.
func (p *Profile) Reorder(key PanelKey, idx int) {
	from := -1
	for i, pref := range p.Panels {
		if pref.Key == key {
			from = i
			break
		}
	}
	if from < 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx > len(p.Panels)-1 {
		idx = len(p.Panels) - 1
	}
	moved := p.Panels[from]
	p.Panels = append(p.Panels[:from], p.Panels[from+1:]...)
	// insert at idx
	p.Panels = append(p.Panels, PanelPref{})
	copy(p.Panels[idx+1:], p.Panels[idx:])
	p.Panels[idx] = moved
	for i := range p.Panels {
		p.Panels[i].Order = i
	}
}

// Toggle flips the visibility of the panel with key. Unknown keys are ignored.
func (p *Profile) Toggle(key PanelKey) {
	for i := range p.Panels {
		if p.Panels[i].Key == key {
			p.Panels[i].Visible = !p.Panels[i].Visible
			return
		}
	}
}

// AssignScreen moves the panel with key to monitor screen (0 principal, 1
// secondary). Unknown keys and out-of-range screens are ignored — the loud
// rejection lives in Validate; this in-place editor stays forgiving so the UI can
// call it freely. Story 9.2.
func (p *Profile) AssignScreen(key PanelKey, screen int) {
	if screen < 0 || screen > 1 {
		return
	}
	for i := range p.Panels {
		if p.Panels[i].Key == key {
			p.Panels[i].Screen = screen
			return
		}
	}
}

// SetTouch sets touch mode. SetScreens sets the monitor count (clamped to 1..2)
// and, when dropping to one monitor, pulls every panel back to the principal
// screen so the layout stays consistent. Story 9.2.
func (p *Profile) SetTouch(on bool) { p.Touch = on }

// SetScreens updates the monitor count, clamping to the valid 1..2 range and
// collapsing panel screens when only one monitor remains.
func (p *Profile) SetScreens(n int) {
	if n < 1 {
		n = 1
	}
	if n > 2 {
		n = 2
	}
	p.Screens = n
	if n == 1 {
		for i := range p.Panels {
			p.Panels[i].Screen = 0
		}
	}
}

// ForScreen returns the visible panels assigned to monitor n, in order. When the
// profile runs a single monitor every visible panel is on screen 0. Used by the
// secondary-monitor wallboard to decide which extra blocks to show. Story 9.2.
func (p *Profile) ForScreen(n int) []PanelPref {
	out := make([]PanelPref, 0, len(p.Panels))
	for _, pref := range p.Panels {
		if pref.Visible && pref.Screen == n {
			out = append(out, pref)
		}
	}
	return out
}

// String helpers used by adapters that persist the enums as text.
func (t Theme) String() string    { return string(t) }
func (d Density) String() string  { return string(d) }
func (k PanelKey) String() string { return string(k) }
