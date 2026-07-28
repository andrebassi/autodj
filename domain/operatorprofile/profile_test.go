package operatorprofile

import "testing"

func TestDefaultCoversAllPanels(t *testing.T) {
	p := Default("usr_1")
	if p.UserID != "usr_1" {
		t.Fatalf("userID = %q, want usr_1", p.UserID)
	}
	if p.Theme != ThemeLight || p.Density != DensityComfortable {
		t.Fatalf("default theme/density = %v/%v", p.Theme, p.Density)
	}
	if len(p.Panels) != len(Panels) {
		t.Fatalf("default has %d panels, want %d", len(p.Panels), len(Panels))
	}
	for i, pref := range p.Panels {
		if pref.Key != Panels[i] {
			t.Errorf("panel %d = %q, want %q", i, pref.Key, Panels[i])
		}
		if !pref.Visible {
			t.Errorf("panel %q should default visible", pref.Key)
		}
		if pref.Order != i {
			t.Errorf("panel %q order = %d, want %d", pref.Key, pref.Order, i)
		}
	}
	// Default must be valid.
	if err := p.Validate(); err != nil {
		t.Fatalf("default profile invalid: %v", err)
	}
}

func TestValidateBadTheme(t *testing.T) {
	p := Default("u")
	p.Theme = "neon"
	if err := p.Validate(); err != ErrBadTheme {
		t.Fatalf("err = %v, want ErrBadTheme", err)
	}
}

func TestValidateBadDensity(t *testing.T) {
	p := Default("u")
	p.Density = "gigante"
	if err := p.Validate(); err != ErrBadDensity {
		t.Fatalf("err = %v, want ErrBadDensity", err)
	}
}

func TestValidateUnknownPanel(t *testing.T) {
	p := Profile{
		UserID:  "u",
		Theme:   ThemeAuto,
		Density: DensityCompact,
		Panels:  []PanelPref{{Key: "inexistente", Visible: true, Order: 0}},
	}
	if err := p.Validate(); err != ErrUnknownPanel {
		t.Fatalf("err = %v, want ErrUnknownPanel", err)
	}
}

func TestValidateDuplicatePanel(t *testing.T) {
	p := Profile{
		UserID:  "u",
		Theme:   ThemeLight,
		Density: DensityComfortable,
		Panels: []PanelPref{
			{Key: PanelQueue, Visible: true, Order: 0},
			{Key: PanelQueue, Visible: false, Order: 1},
		},
	}
	if err := p.Validate(); err != ErrDupPanel {
		t.Fatalf("err = %v, want ErrDupPanel", err)
	}
}

func TestValidateNormalizesOrder(t *testing.T) {
	p := Profile{
		UserID:  "u",
		Theme:   ThemeDark,
		Density: DensityComfortable,
		Panels: []PanelPref{
			{Key: PanelCartwall, Visible: true, Order: 50},
			{Key: PanelQueue, Visible: true, Order: 10},
			{Key: PanelLibrary, Visible: false, Order: 30},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	want := []PanelKey{PanelQueue, PanelLibrary, PanelCartwall}
	for i, pref := range p.Panels {
		if pref.Key != want[i] {
			t.Errorf("panel %d = %q, want %q", i, pref.Key, want[i])
		}
		if pref.Order != i {
			t.Errorf("panel %q order = %d, want %d", pref.Key, pref.Order, i)
		}
	}
}

func TestReorder(t *testing.T) {
	p := Default("u") // fila, biblioteca, cartwall, spotify, aidj
	p.Reorder(PanelAIDJ, 0)
	if p.Panels[0].Key != PanelAIDJ {
		t.Fatalf("after reorder first = %q, want aidj", p.Panels[0].Key)
	}
	for i := range p.Panels {
		if p.Panels[i].Order != i {
			t.Errorf("panel %d order = %d", i, p.Panels[i].Order)
		}
	}
	// move to end
	p.Reorder(PanelAIDJ, 99)
	if p.Panels[len(p.Panels)-1].Key != PanelAIDJ {
		t.Fatalf("reorder to end failed")
	}
	// negative clamps to 0
	p.Reorder(PanelSpotify, -5)
	if p.Panels[0].Key != PanelSpotify {
		t.Fatalf("reorder negative should clamp to front")
	}
	// unknown key: no-op
	before := len(p.Panels)
	p.Reorder("nada", 2)
	if len(p.Panels) != before {
		t.Fatalf("reorder unknown mutated list")
	}
}

func TestToggle(t *testing.T) {
	p := Default("u")
	p.Toggle(PanelLibrary)
	for _, pref := range p.Panels {
		if pref.Key == PanelLibrary && pref.Visible {
			t.Fatalf("library should be hidden after toggle")
		}
	}
	p.Toggle(PanelLibrary)
	for _, pref := range p.Panels {
		if pref.Key == PanelLibrary && !pref.Visible {
			t.Fatalf("library should be visible after second toggle")
		}
	}
	// unknown key: no-op
	p.Toggle("nada")
}

func TestStringHelpers(t *testing.T) {
	if ThemeDark.String() != "escuro" {
		t.Errorf("theme string %q", ThemeDark.String())
	}
	if DensityCompact.String() != "compacto" {
		t.Errorf("density string %q", DensityCompact.String())
	}
	if PanelQueue.String() != "fila" {
		t.Errorf("panel string %q", PanelQueue.String())
	}
}

func TestValidRoleLists(t *testing.T) {
	if !validTheme(ThemeAuto) || validTheme("x") {
		t.Errorf("validTheme wrong")
	}
	if !validDensity(DensityCompact) || validDensity("x") {
		t.Errorf("validDensity wrong")
	}
}

// ---- Story 9.2: touch + dois monitores ----

func TestDefaultSingleMonitorNoTouch(t *testing.T) {
	p := Default("u")
	if p.Touch {
		t.Errorf("default should not enable touch")
	}
	if p.Screens != 1 {
		t.Errorf("default screens = %d, want 1", p.Screens)
	}
	for _, pref := range p.Panels {
		if pref.Screen != 0 {
			t.Errorf("panel %q default screen = %d, want 0", pref.Key, pref.Screen)
		}
	}
}

func TestValidateScreensBackCompat(t *testing.T) {
	// A profile decoded from a pre-9.2 store has Screens == 0; Validate coerces to 1.
	p := Profile{UserID: "u", Theme: ThemeDark, Density: DensityComfortable}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if p.Screens != 1 {
		t.Fatalf("Screens coerced to %d, want 1", p.Screens)
	}
}

func TestValidateBadScreens(t *testing.T) {
	p := Default("u")
	p.Screens = 3
	if err := p.Validate(); err != ErrBadScreens {
		t.Fatalf("err = %v, want ErrBadScreens", err)
	}
}

func TestValidateBadScreen(t *testing.T) {
	p := Default("u")
	p.Screens = 2
	p.Panels[0].Screen = 2 // only 0/1 allowed
	if err := p.Validate(); err != ErrBadScreen {
		t.Fatalf("err = %v, want ErrBadScreen", err)
	}
	// negative also rejected
	p.Panels[0].Screen = -1
	if err := p.Validate(); err != ErrBadScreen {
		t.Fatalf("err = %v, want ErrBadScreen (negative)", err)
	}
}

func TestNormalizeCollapsesScreenOnSingleMonitor(t *testing.T) {
	p := Default("u")
	p.Screens = 2
	p.AssignScreen(PanelLibrary, 1)
	if err := p.Validate(); err != nil {
		t.Fatalf("validate dual: %v", err)
	}
	// library must still be on screen 1 with two monitors
	if got := screenOf(p, PanelLibrary); got != 1 {
		t.Fatalf("dual: library screen = %d, want 1", got)
	}
	// drop to one monitor → normalize pulls it back to 0
	p.Screens = 1
	if err := p.Validate(); err != nil {
		t.Fatalf("validate single: %v", err)
	}
	if got := screenOf(p, PanelLibrary); got != 0 {
		t.Fatalf("single: library screen = %d, want 0", got)
	}
}

func TestAssignScreen(t *testing.T) {
	p := Default("u")
	p.AssignScreen(PanelCartwall, 1)
	if got := screenOf(p, PanelCartwall); got != 1 {
		t.Fatalf("assign screen = %d, want 1", got)
	}
	// out-of-range ignored
	p.AssignScreen(PanelCartwall, 5)
	if got := screenOf(p, PanelCartwall); got != 1 {
		t.Fatalf("out-of-range assign changed screen to %d", got)
	}
	p.AssignScreen(PanelCartwall, -1)
	if got := screenOf(p, PanelCartwall); got != 1 {
		t.Fatalf("negative assign changed screen to %d", got)
	}
	// unknown key ignored
	p.AssignScreen("nada", 1)
}

func TestSetTouchAndScreens(t *testing.T) {
	p := Default("u")
	p.SetTouch(true)
	if !p.Touch {
		t.Fatalf("SetTouch(true) not applied")
	}
	p.SetScreens(2)
	p.AssignScreen(PanelSpotify, 1)
	if p.Screens != 2 {
		t.Fatalf("SetScreens(2) = %d", p.Screens)
	}
	// clamp high
	p.SetScreens(9)
	if p.Screens != 2 {
		t.Fatalf("SetScreens(9) clamp = %d, want 2", p.Screens)
	}
	// clamp low + collapse screens
	p.SetScreens(0)
	if p.Screens != 1 {
		t.Fatalf("SetScreens(0) clamp = %d, want 1", p.Screens)
	}
	if got := screenOf(p, PanelSpotify); got != 0 {
		t.Fatalf("collapse to single left spotify on screen %d", got)
	}
}

func TestForScreen(t *testing.T) {
	p := Default("u")
	p.SetScreens(2)
	p.AssignScreen(PanelLibrary, 1)
	p.AssignScreen(PanelCartwall, 1)
	p.Toggle(PanelCartwall) // hide it — ForScreen returns only visible
	s0 := p.ForScreen(0)
	s1 := p.ForScreen(1)
	// screen 1 visible: only library (cartwall hidden)
	if len(s1) != 1 || s1[0].Key != PanelLibrary {
		t.Fatalf("ForScreen(1) = %v, want [library]", s1)
	}
	// screen 0 visible: fila, spotify, aidj (library moved, cartwall hidden)
	if len(s0) != 3 {
		t.Fatalf("ForScreen(0) len = %d, want 3", len(s0))
	}
	for _, pref := range s0 {
		if pref.Key == PanelLibrary || pref.Key == PanelCartwall {
			t.Fatalf("ForScreen(0) wrongly includes %q", pref.Key)
		}
	}
}

// screenOf finds a panel's assigned monitor by key (test helper).
func screenOf(p Profile, key PanelKey) int {
	for _, pref := range p.Panels {
		if pref.Key == key {
			return pref.Screen
		}
	}
	return -1
}
