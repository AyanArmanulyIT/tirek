package identity

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h := PasswordHasher{}
	encoded, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("unexpected PHC prefix: %s", encoded)
	}
	// Two hashes of the same password differ (random salt).
	other, _ := h.Hash("correct horse battery staple")
	if encoded == other {
		t.Error("two hashes of the same password are identical; salt not random")
	}

	ok, err := h.Verify("correct horse battery staple", encoded)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Error("correct password did not verify")
	}

	ok, err = h.Verify("wrong password", encoded)
	if err != nil {
		t.Fatalf("verify wrong password: %v", err)
	}
	if ok {
		t.Error("wrong password verified")
	}
}

func TestPasswordVerifyMalformed(t *testing.T) {
	h := PasswordHasher{}
	for _, bad := range []string{
		"",
		"$argon2id",
		"$argon2id$v=19$m=19456,t=2,p=1$AAAA",
		"$md5$v=19$m=19456,t=2,p=1$c2FsdA==$aGFzaA==",
		"$argon2id$v=18$m=19456,t=2,p=1$c2FsdA==$aGFzaA==",
		"$argon2id$v=19$m=0,t=2,p=1$c2FsdA==$aGFzaA==",
		"$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA==",
	} {
		if _, err := h.Verify("password", bad); err == nil {
			t.Errorf("Verify(%q) succeeded, want error", bad)
		}
	}
}

func TestPasswordRejectsEmpty(t *testing.T) {
	h := PasswordHasher{}
	for _, pw := range []string{"", "   "} {
		encoded, err := h.Hash(pw)
		if err == nil {
			t.Errorf("Hash(%q) = %s, want error", pw, encoded)
		}
	}
}