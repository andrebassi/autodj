package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"autodj/domain/identity"
	"autodj/domain/operatorprofile"
	"autodj/ports/inbound"
)

// fakeProfile is a configurable ProfileService stub. getProfile/getErr drive Get;
// saveErr drives Save. gotUserID/gotActor/gotProfile record the last call.
type fakeProfile struct {
	getProfile operatorprofile.Profile
	getErr     error
	saveErr    error
	saveOut    operatorprofile.Profile

	gotUserID  string
	gotActorID string
	gotProfile operatorprofile.Profile
}

func (f *fakeProfile) Get(_ context.Context, userID string) (operatorprofile.Profile, error) {
	f.gotUserID = userID
	if f.getErr != nil {
		return operatorprofile.Profile{}, f.getErr
	}
	return f.getProfile, nil
}

func (f *fakeProfile) Save(_ context.Context, actor identity.User, p operatorprofile.Profile) (operatorprofile.Profile, error) {
	f.gotActorID, f.gotProfile = actor.ID, p
	if f.saveErr != nil {
		return operatorprofile.Profile{}, f.saveErr
	}
	if f.saveOut.UserID == "" {
		return p, nil
	}
	return f.saveOut, nil
}

// withProfile wires a router with both auth (owner resolution) and the profile svc.
func withProfile(auth inbound.AuthService, prof inbound.ProfileService) http.Handler {
	return New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil,
		WithAuth(auth), WithProfile(prof))
}

func mkUser(id string) identity.User {
	u, _ := identity.NewUser(id, "op", identity.RoleOperator, "h")
	return u
}

func TestGetProfileOK(t *testing.T) {
	me := mkUser("usr_1")
	prof := &fakeProfile{getProfile: operatorprofile.Default("usr_1")}
	h := withProfile(&fakeAuth{meUser: me}, prof)
	rec := doAuth(t, h, http.MethodGet, "/api/v1/profile/me", "", "T:op")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if prof.gotUserID != "usr_1" {
		t.Fatalf("Get called with %q, want usr_1", prof.gotUserID)
	}
	var got operatorprofile.Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Theme != operatorprofile.ThemeLight || len(got.Panels) != len(operatorprofile.Panels) {
		t.Fatalf("profile=%+v", got)
	}
}

func TestGetProfileBadToken(t *testing.T) {
	prof := &fakeProfile{}
	h := withProfile(&fakeAuth{meErr: inbound.ErrBadToken}, prof)
	rec := doAuth(t, h, http.MethodGet, "/api/v1/profile/me", "", "")
	if rec.Code != 401 {
		t.Fatalf("bad token should 401, got %d", rec.Code)
	}
}

func TestGetProfileRepoError(t *testing.T) {
	prof := &fakeProfile{getErr: context.DeadlineExceeded}
	h := withProfile(&fakeAuth{meUser: mkUser("usr_1")}, prof)
	rec := doAuth(t, h, http.MethodGet, "/api/v1/profile/me", "", "T:op")
	if rec.Code != 500 {
		t.Fatalf("repo error should 500, got %d", rec.Code)
	}
}

func TestPutProfileOK(t *testing.T) {
	me := mkUser("usr_1")
	prof := &fakeProfile{}
	h := withProfile(&fakeAuth{meUser: me}, prof)
	// body carries a foreign userId — the handler must override it with the token's
	body := `{"userId":"usr_hacker","theme":"claro","density":"compacto","panels":[{"key":"fila","visible":true,"order":0}]}`
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me", body, "T:op")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if prof.gotProfile.UserID != "usr_1" {
		t.Fatalf("owner not overridden: got %q, want usr_1", prof.gotProfile.UserID)
	}
	if prof.gotActorID != "usr_1" {
		t.Fatalf("actor=%q, want usr_1", prof.gotActorID)
	}
	if prof.gotProfile.Theme != operatorprofile.ThemeLight {
		t.Fatalf("theme=%v", prof.gotProfile.Theme)
	}
}

func TestPutProfileBadBody(t *testing.T) {
	prof := &fakeProfile{}
	h := withProfile(&fakeAuth{meUser: mkUser("usr_1")}, prof)
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me", `{`, "T:op")
	if rec.Code != 400 {
		t.Fatalf("malformed body should 400, got %d", rec.Code)
	}
}

func TestPutProfileBadToken(t *testing.T) {
	prof := &fakeProfile{}
	h := withProfile(&fakeAuth{meErr: inbound.ErrBadToken}, prof)
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me",
		`{"theme":"claro","density":"compacto","panels":[]}`, "")
	if rec.Code != 401 {
		t.Fatalf("bad token should 401, got %d", rec.Code)
	}
}

func TestPutProfileValidationError(t *testing.T) {
	prof := &fakeProfile{saveErr: operatorprofile.ErrBadTheme}
	h := withProfile(&fakeAuth{meUser: mkUser("usr_1")}, prof)
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me",
		`{"theme":"neon","density":"compacto","panels":[]}`, "T:op")
	if rec.Code != 422 {
		t.Fatalf("bad theme should 422, got %d", rec.Code)
	}
}

func TestPutProfileForbidden(t *testing.T) {
	prof := &fakeProfile{saveErr: inbound.ErrForbidden}
	h := withProfile(&fakeAuth{meUser: mkUser("usr_1")}, prof)
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me",
		`{"theme":"claro","density":"compacto","panels":[]}`, "T:op")
	if rec.Code != 403 {
		t.Fatalf("forbidden should 403, got %d", rec.Code)
	}
}

// Story 9.2: touch/screens/screen round-trip PUT→handler and GET→JSON.
func TestPutProfileTouchScreensRoundTrip(t *testing.T) {
	me := mkUser("usr_1")
	prof := &fakeProfile{}
	h := withProfile(&fakeAuth{meUser: me}, prof)
	body := `{"theme":"escuro","density":"confortavel","touch":true,"screens":2,` +
		`"panels":[{"key":"fila","visible":true,"order":0,"screen":0},` +
		`{"key":"cartwall","visible":true,"order":1,"screen":1}]}`
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me", body, "T:op")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	if !prof.gotProfile.Touch || prof.gotProfile.Screens != 2 {
		t.Fatalf("touch/screens not decoded: %v/%d", prof.gotProfile.Touch, prof.gotProfile.Screens)
	}
	var cart int = -1
	for _, p := range prof.gotProfile.Panels {
		if p.Key == operatorprofile.PanelCartwall {
			cart = p.Screen
		}
	}
	if cart != 1 {
		t.Fatalf("cartwall screen decoded = %d, want 1", cart)
	}
}

func TestGetProfileTouchScreensSerialized(t *testing.T) {
	me := mkUser("usr_1")
	p := operatorprofile.Default("usr_1")
	p.SetTouch(true)
	p.SetScreens(2)
	p.AssignScreen(operatorprofile.PanelSpotify, 1)
	prof := &fakeProfile{getProfile: p}
	h := withProfile(&fakeAuth{meUser: me}, prof)
	rec := doAuth(t, h, http.MethodGet, "/api/v1/profile/me", "", "T:op")
	if rec.Code != 200 {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var got operatorprofile.Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Touch || got.Screens != 2 {
		t.Fatalf("serialized touch/screens = %v/%d, want true/2", got.Touch, got.Screens)
	}
	for _, pref := range got.Panels {
		if pref.Key == operatorprofile.PanelSpotify && pref.Screen != 1 {
			t.Fatalf("spotify screen serialized = %d, want 1", pref.Screen)
		}
	}
}

func TestPutProfileBadScreens(t *testing.T) {
	prof := &fakeProfile{saveErr: operatorprofile.ErrBadScreens}
	h := withProfile(&fakeAuth{meUser: mkUser("usr_1")}, prof)
	rec := doAuth(t, h, http.MethodPut, "/api/v1/profile/me",
		`{"theme":"escuro","density":"compacto","screens":3,"panels":[]}`, "T:op")
	if rec.Code != 422 {
		t.Fatalf("bad screens should 422, got %d", rec.Code)
	}
}

func TestProfileRoutesAbsentWithoutOption(t *testing.T) {
	// profile wired but no auth → routes not registered (needs both)
	h := New(fakeLib{}, fakePlay{}, fakeContent{}, fakeMix{}, fakeSched{}, nil,
		WithProfile(&fakeProfile{}))
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/profile/me"},
		{http.MethodPut, "/api/v1/profile/me"},
	} {
		if rec := do(t, h, tc.method, tc.path, ""); rec.Code != 404 {
			t.Fatalf("%s %s should 404 without WithAuth, got %d", tc.method, tc.path, rec.Code)
		}
	}
}
