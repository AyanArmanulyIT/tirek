package identity

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func newManager() TokenManager {
	return TokenManager{
		AccessSecret:  []byte("unit-test-access-secret-0123456789abcdef"),
		RefreshSecret: []byte("unit-test-refresh-secret-0123456789abcdef"),
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    720 * time.Hour,
	}
}

func TestIssueAndParseAccessToken(t *testing.T) {
	m := newManager()
	userID, orgID, sessionID := uuid.New(), uuid.New(), uuid.New()
	raw, exp, err := m.IssueAccessToken(userID, orgID, sessionID, "owner")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if raw == "" {
		t.Fatal("empty token")
	}
	if !exp.After(time.Now()) {
		t.Errorf("expiry %v not in future", exp)
	}
	if !strings.HasPrefix(raw, "eyJ") {
		t.Errorf("token does not look like a JWT: %q", raw[:min(len(raw), 8)])
	}

	claims, err := m.ParseAccessToken(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Subject != userID.String() {
		t.Errorf("subject = %s, want %s", claims.Subject, userID)
	}
	if claims.OrgID != orgID.String() || claims.SessionID != sessionID.String() || claims.Role != "owner" {
		t.Errorf("unexpected claims: org=%s session=%s role=%s", claims.OrgID, claims.SessionID, claims.Role)
	}
}

func TestParseAccessTokenRejects(t *testing.T) {
	m := newManager()
	userID, orgID, sessionID := uuid.New(), uuid.New(), uuid.New()

	// Wrong secret.
	wrong := TokenManager{AccessSecret: []byte("totally-different-secret-0123456789abcdef")}
	raw, _, _ := wrong.IssueAccessToken(userID, orgID, sessionID, "owner")
	if _, err := m.ParseAccessToken(raw); err == nil {
		t.Error("token signed with a different secret parsed successfully")
	}

	// Expired.
	m2 := TokenManager{AccessSecret: []byte("unit-test-access-secret-0123456789abcdef"), AccessTTL: -time.Minute}
	expired, _, _ := m2.IssueAccessToken(userID, orgID, sessionID, "owner")
	if _, err := m.ParseAccessToken(expired); err == nil {
		t.Error("expired token parsed successfully")
	}

	// alg=none confusion attack.
	claims := Claims{
		OrgID: orgID.String(), Role: "owner", SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := m.ParseAccessToken(none); err == nil {
		t.Error("alg=none token parsed successfully")
	}

	// Missing required claims.
	bad := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: userID.String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, bad).SignedString(m.AccessSecret)
	if _, err := m.ParseAccessToken(tok); err == nil {
		t.Error("token missing org/session/role claims parsed successfully")
	}

	// Non-UUID subject.
	bad2 := Claims{
		OrgID: orgID.String(), Role: "owner", SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "not-a-uuid", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok2, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, bad2).SignedString(m.AccessSecret)
	if _, err := m.ParseAccessToken(tok2); err == nil {
		t.Error("token with non-UUID subject parsed successfully")
	}
}

func TestRefreshTokenHash(t *testing.T) {
	raw1, hash1, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	raw2, hash2, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	if raw1 == raw2 {
		t.Error("two refresh tokens are identical")
	}
	if hash1 == hash2 {
		t.Error("two refresh token hashes are identical")
	}
	if hash1 != HashRefreshToken(raw1) {
		t.Error("stored hash does not match raw token")
	}
	if len(raw1) != 43 { // 32 bytes base64url-encoded, no padding.
		t.Errorf("raw refresh token length = %d, want 43", len(raw1))
	}
	if hash1 == raw1 {
		t.Error("refresh token is stored in plaintext")
	}
}