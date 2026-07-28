package jwt

import (
	"errors"
	"testing"
	"time"

	"autodj/domain/identity"
)

func admin() identity.User {
	u, _ := identity.NewUser("u1", "root", identity.RoleAdmin, "h")
	return u
}

func TestIssueParseRoundTrip(t *testing.T) {
	iss := New([]byte("secret"), time.Hour)
	tok, err := iss.Issue(admin())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := iss.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != "u1" || claims.Role != identity.RoleAdmin {
		t.Fatalf("claims %+v", claims)
	}
}

func TestParseMalformed(t *testing.T) {
	iss := New([]byte("secret"), time.Hour)
	for _, tok := range []string{"", "a.b", "a.b.c.d", "!!.??.$$"} {
		if _, err := iss.Parse(tok); !errors.Is(err, ErrMalformed) {
			t.Errorf("Parse(%q) err=%v want ErrMalformed", tok, err)
		}
	}
}

func TestParseBadPayloadJSON(t *testing.T) {
	iss := New([]byte("secret"), time.Hour)
	// header "{}", payload "not-json", correctly signed → payload unmarshal fails
	signing := enc([]byte("{}")) + "." + enc([]byte("not-json"))
	tok := signing + "." + enc(iss.sign(signing))
	if _, err := iss.Parse(tok); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err=%v want ErrMalformed", err)
	}
}

func TestParseTampered(t *testing.T) {
	iss := New([]byte("secret"), time.Hour)
	tok, _ := iss.Issue(admin())
	// flip a char in the payload segment
	other := New([]byte("different-secret"), time.Hour)
	if _, err := other.Parse(tok); !errors.Is(err, ErrSignature) {
		t.Fatalf("err=%v want ErrSignature", err)
	}
}

func TestParseExpired(t *testing.T) {
	iss := New([]byte("secret"), time.Hour)
	// freeze issue time in the past so the token is already expired
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	iss.now = func() time.Time { return base }
	tok, _ := iss.Issue(admin())
	iss.now = func() time.Time { return base.Add(2 * time.Hour) }
	if _, err := iss.Parse(tok); !errors.Is(err, ErrExpired) {
		t.Fatalf("err=%v want ErrExpired", err)
	}
}

func TestParseBadSigB64(t *testing.T) {
	iss := New([]byte("secret"), time.Hour)
	tok, _ := iss.Issue(admin())
	parts := tok[:len(tok)-3] + "!!!" // corrupt the signature segment b64
	if _, err := iss.Parse(parts); !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrSignature) {
		t.Fatalf("err=%v want malformed/signature", err)
	}
}
