package argon2id

import "testing"

// cheap params keep the tests fast while exercising the same code paths.
var testParams = Params{Memory: 8 * 1024, Time: 1, Threads: 1, KeyLen: 16, SaltLen: 8}

func TestHashVerifyRoundTrip(t *testing.T) {
	h := NewWith(testParams)
	hash, err := h.Hash("s3nha-forte")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "s3nha-forte" {
		t.Fatal("hash must differ from plaintext")
	}
	if !h.Verify("s3nha-forte", hash) {
		t.Fatal("Verify should accept the right password")
	}
	if h.Verify("errada", hash) {
		t.Fatal("Verify should reject the wrong password")
	}
}

func TestHashIsSalted(t *testing.T) {
	h := NewWith(testParams)
	a, _ := h.Hash("same")
	b, _ := h.Hash("same")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
	// but both verify
	if !h.Verify("same", a) || !h.Verify("same", b) {
		t.Fatal("both salted hashes must verify")
	}
}

func TestVerifyMalformed(t *testing.T) {
	h := NewWith(testParams)
	bad := []string{
		"",
		"not-a-hash",
		"$argon2i$v=19$m=8,t=1,p=1$c2FsdA$a2V5",           // wrong variant
		"$argon2id$v=99$m=8,t=1,p=1$c2FsdA$a2V5",          // wrong version
		"$argon2id$v=19$bad$c2FsdA$a2V5",                  // bad params
		"$argon2id$v=19$m=8,t=1,p=1$!!!$a2V5",             // bad salt b64
		"$argon2id$v=19$m=8,t=1,p=1$c2FsdA$!!!",           // bad key b64
		"$argon2id$v=19$m=8,t=1,p=1$c2FsdA",               // too few parts
	}
	for _, s := range bad {
		if h.Verify("x", s) {
			t.Errorf("Verify accepted malformed hash %q", s)
		}
	}
}

func TestNewDefaults(t *testing.T) {
	h := New()
	if h.p != DefaultParams {
		t.Fatalf("New params %+v want defaults", h.p)
	}
}
