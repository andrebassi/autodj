// Package identity models access control for the station: who a user is, which
// role they hold, and which permissions that role grants. It is pure — no I/O,
// no crypto, no deps beyond the standard library.
//
// Passwords never live here. User.PassHash is an opaque token produced and
// checked by a PasswordHasher port (argon2id adapter); the domain only decides,
// via a fixed role→permission matrix, whether a role Can perform a permission.
// Home use means few accounts, but the matrix is the single source of truth for
// every gate in the app and the REST middleware.
package identity

import (
	"errors"
	"strings"
)

// Errors returned when building a user.
var (
	// ErrEmptyName rejects a user with no name.
	ErrEmptyName = errors.New("identity: name must not be empty")
	// ErrBadRole rejects a user whose role is not one of the known roles.
	ErrBadRole = errors.New("identity: unknown role")
)

// Role is what a user is allowed to be. The set is fixed and small: one owner-
// level admin, the day-to-day operator, and the on-air announcer.
type Role string

// The known roles, from most to least privileged.
const (
	// RoleAdmin owns the station: full access, including managing users.
	RoleAdmin Role = "admin"
	// RoleOperator runs the playout: playback, cartwall, reports — no user mgmt.
	RoleOperator Role = "operador"
	// RoleAnnouncer speaks on air: playback control + cartwall only.
	RoleAnnouncer Role = "locutor"
)

// Permission is one guarded capability. Permissions are coarse — a whole feature
// area — so the matrix stays readable and the REST middleware maps cleanly.
type Permission string

// The known permissions.
const (
	// PermLibraryWrite covers adding, editing and deleting library media.
	PermLibraryWrite Permission = "library.write"
	// PermScheduleWrite covers editing clocks, grades and events.
	PermScheduleWrite Permission = "schedule.write"
	// PermPlaybackControl covers play/skip/enqueue on the live queue.
	PermPlaybackControl Permission = "playback.control"
	// PermCartwallFire covers firing cartwall buttons on air.
	PermCartwallFire Permission = "cartwall.fire"
	// PermReportsRead covers reading play logs and reports.
	PermReportsRead Permission = "reports.read"
	// PermUsersManage covers creating, editing and deleting user accounts.
	PermUsersManage Permission = "users.manage"
)

// Roles lists the known roles in privilege order. Useful for validation and UI.
var Roles = []Role{RoleAdmin, RoleOperator, RoleAnnouncer}

// Permissions lists every known permission. Useful for admin UIs and tests.
var Permissions = []Permission{
	PermLibraryWrite,
	PermScheduleWrite,
	PermPlaybackControl,
	PermCartwallFire,
	PermReportsRead,
	PermUsersManage,
}

// matrix is the fixed role→permission grant table — the heart of RBAC. Admin
// holds every permission; operator runs playout and reads reports but never
// manages users; announcer only drives playback and fires cartwall. A table-
// driven test covers every cell.
var matrix = map[Role]map[Permission]bool{
	RoleAdmin: {
		PermLibraryWrite:    true,
		PermScheduleWrite:   true,
		PermPlaybackControl: true,
		PermCartwallFire:    true,
		PermReportsRead:     true,
		PermUsersManage:     true,
	},
	RoleOperator: {
		PermLibraryWrite:    true,
		PermScheduleWrite:   true,
		PermPlaybackControl: true,
		PermCartwallFire:    true,
		PermReportsRead:     true,
	},
	RoleAnnouncer: {
		PermPlaybackControl: true,
		PermCartwallFire:    true,
	},
}

// ValidRole reports whether r is one of the known roles.
func ValidRole(r Role) bool {
	_, ok := matrix[r]
	return ok
}

// Can reports whether a role is granted a permission by the fixed matrix. An
// unknown role grants nothing.
func Can(role Role, perm Permission) bool {
	return matrix[role][perm]
}

// User is one account: a stable id, a display name, the role that decides access
// and an opaque password hash produced by the PasswordHasher port. The hash is
// never inspected here and must never be logged.
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     Role   `json:"role"`
	PassHash string `json:"-"`
}

// NewUser builds a user, validating the name and role. The password hash is
// supplied by the caller (already produced by the hasher port); an empty hash is
// allowed only for callers that set it later, so it is not validated here.
func NewUser(id, name string, role Role, passHash string) (User, error) {
	if strings.TrimSpace(name) == "" {
		return User{}, ErrEmptyName
	}
	if !ValidRole(role) {
		return User{}, ErrBadRole
	}
	return User{ID: id, Name: name, Role: role, PassHash: passHash}, nil
}

// Can reports whether this user's role grants the permission.
func (u User) Can(perm Permission) bool {
	return Can(u.Role, perm)
}
