package spotify

import (
	"testing"
	"time"
)

var base = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

func TestAccount_Connected(t *testing.T) {
	if (Account{}).Connected() {
		t.Fatal("zero account must not be connected")
	}
	if !(Account{AccessToken: "tok"}).Connected() {
		t.Fatal("account with token must be connected")
	}
}

func TestAccount_Premium(t *testing.T) {
	cases := map[string]bool{
		"premium": true,
		"free":    false,
		"open":    false,
		"":        false,
	}
	for product, want := range cases {
		if got := (Account{Product: product}).Premium(); got != want {
			t.Fatalf("Premium(%q) = %v, want %v", product, got, want)
		}
	}
}

func TestAccount_Expired(t *testing.T) {
	// Zero ExpiresAt => expired.
	if !(Account{}).Expired(base) {
		t.Fatal("zero ExpiresAt must be expired")
	}
	// Before expiry => not expired.
	a := Account{ExpiresAt: base.Add(time.Hour)}
	if a.Expired(base) {
		t.Fatal("token valid an hour out must not be expired")
	}
	// Exact instant => expired (inclusive).
	if !a.Expired(base.Add(time.Hour)) {
		t.Fatal("token at exact expiry must be expired")
	}
	// After expiry => expired.
	if !a.Expired(base.Add(2 * time.Hour)) {
		t.Fatal("token past expiry must be expired")
	}
}

func TestAccount_NeedsRefresh(t *testing.T) {
	// Zero ExpiresAt => needs refresh.
	if !(Account{}).NeedsRefresh(base) {
		t.Fatal("zero ExpiresAt must need refresh")
	}
	a := Account{ExpiresAt: base.Add(time.Hour)}
	// Well before window => no refresh.
	if a.NeedsRefresh(base) {
		t.Fatal("token an hour out must not need refresh")
	}
	// Just inside the refresh window => refresh.
	if !a.NeedsRefresh(base.Add(time.Hour - refreshWindow/2)) {
		t.Fatal("token inside refresh window must need refresh")
	}
	// Exactly at window boundary => refresh (inclusive).
	if !a.NeedsRefresh(base.Add(time.Hour - refreshWindow)) {
		t.Fatal("token at refresh boundary must need refresh")
	}
	// Past expiry => refresh.
	if !a.NeedsRefresh(base.Add(2 * time.Hour)) {
		t.Fatal("expired token must need refresh")
	}
}

func TestAccount_HasScope(t *testing.T) {
	a := Account{Scopes: []string{"streaming", "user-read-private"}}
	if !a.HasScope("streaming") {
		t.Fatal("granted scope must be present")
	}
	if a.HasScope("user-modify-playback-state") {
		t.Fatal("ungranted scope must be absent")
	}
	if (Account{}).HasScope("streaming") {
		t.Fatal("no scopes => none present")
	}
}

func TestPendingAuth_Expired(t *testing.T) {
	// Zero CreatedAt => expired.
	if !(PendingAuth{}).Expired(base) {
		t.Fatal("zero CreatedAt must be expired")
	}
	p := PendingAuth{State: "s", Verifier: "v", CreatedAt: base}
	// Within TTL => not expired.
	if p.Expired(base.Add(pendingTTL - time.Second)) {
		t.Fatal("fresh pending must not be expired")
	}
	// Exactly at TTL => not expired (strictly greater).
	if p.Expired(base.Add(pendingTTL)) {
		t.Fatal("pending at exact TTL must not be expired")
	}
	// Past TTL => expired.
	if !p.Expired(base.Add(pendingTTL + time.Second)) {
		t.Fatal("pending past TTL must be expired")
	}
}
