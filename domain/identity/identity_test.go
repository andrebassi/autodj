package identity

import (
	"errors"
	"testing"
)

func TestCanMatrix(t *testing.T) {
	// every (role, perm) cell of the fixed matrix, asserted explicitly.
	cases := []struct {
		role Role
		perm Permission
		want bool
	}{
		// admin: everything
		{RoleAdmin, PermLibraryWrite, true},
		{RoleAdmin, PermScheduleWrite, true},
		{RoleAdmin, PermPlaybackControl, true},
		{RoleAdmin, PermCartwallFire, true},
		{RoleAdmin, PermReportsRead, true},
		{RoleAdmin, PermUsersManage, true},
		// operator: playout + reports, no user mgmt
		{RoleOperator, PermLibraryWrite, true},
		{RoleOperator, PermScheduleWrite, true},
		{RoleOperator, PermPlaybackControl, true},
		{RoleOperator, PermCartwallFire, true},
		{RoleOperator, PermReportsRead, true},
		{RoleOperator, PermUsersManage, false},
		// announcer: playback + cartwall only
		{RoleAnnouncer, PermLibraryWrite, false},
		{RoleAnnouncer, PermScheduleWrite, false},
		{RoleAnnouncer, PermPlaybackControl, true},
		{RoleAnnouncer, PermCartwallFire, true},
		{RoleAnnouncer, PermReportsRead, false},
		{RoleAnnouncer, PermUsersManage, false},
	}
	for _, c := range cases {
		if got := Can(c.role, c.perm); got != c.want {
			t.Errorf("Can(%q,%q)=%v want %v", c.role, c.perm, got, c.want)
		}
	}
}

func TestCanUnknownRole(t *testing.T) {
	if Can(Role("root"), PermPlaybackControl) {
		t.Fatal("unknown role must grant nothing")
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range Roles {
		if !ValidRole(r) {
			t.Errorf("ValidRole(%q)=false want true", r)
		}
	}
	if ValidRole(Role("root")) {
		t.Fatal("ValidRole(root)=true want false")
	}
}

func TestPermissionsCoverMatrix(t *testing.T) {
	// admin must grant every permission listed in Permissions — guards drift.
	for _, p := range Permissions {
		if !Can(RoleAdmin, p) {
			t.Errorf("admin missing permission %q", p)
		}
	}
}

func TestNewUser(t *testing.T) {
	u, err := NewUser("u1", "Ana", RoleOperator, "hash123")
	if err != nil {
		t.Fatalf("NewUser: %v", err)
	}
	if u.ID != "u1" || u.Name != "Ana" || u.Role != RoleOperator || u.PassHash != "hash123" {
		t.Fatalf("NewUser produced %+v", u)
	}
}

func TestNewUserEmptyName(t *testing.T) {
	if _, err := NewUser("u1", "   ", RoleAdmin, "h"); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("err=%v want ErrEmptyName", err)
	}
}

func TestNewUserBadRole(t *testing.T) {
	if _, err := NewUser("u1", "Ana", Role("root"), "h"); !errors.Is(err, ErrBadRole) {
		t.Fatalf("err=%v want ErrBadRole", err)
	}
}

func TestUserCan(t *testing.T) {
	u, _ := NewUser("u1", "Ana", RoleAnnouncer, "h")
	if !u.Can(PermCartwallFire) {
		t.Error("announcer should fire cartwall")
	}
	if u.Can(PermLibraryWrite) {
		t.Error("announcer must not write library")
	}
}
