package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims are the Tirek access-token claims (see ADR-007).
type Claims struct {
	OrgID     string `json:"org_id"`
	Role      string `json:"role"`
	SessionID string `json:"session_id"`
	jwt.RegisteredClaims
}

// TokenManager signs and validates JWT access tokens and generates the opaque
// rotating refresh tokens.
type TokenManager struct {
	AccessSecret  []byte
	AccessTTL     time.Duration
	RefreshSecret []byte
	RefreshTTL    time.Duration
}

// IssueAccessToken signs a new HS256 access token and returns the encoded
// token plus its expiry time.
func (m TokenManager) IssueAccessToken(userID, orgID, sessionID uuid.UUID, role string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(m.AccessTTL)
	claims := Claims{
		OrgID:     orgID.String(),
		Role:      role,
		SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.AccessSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseAccessToken validates a bearer token: signature, algorithm, and expiry.
func (m TokenManager) ParseAccessToken(raw string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %q", t.Method.Alg())
		}
		return m.AccessSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid access token")
	}
	if claims.Subject == "" || claims.OrgID == "" || claims.SessionID == "" || claims.Role == "" {
		return nil, errors.New("access token missing required claims")
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return nil, errors.New("access token subject is not a UUID")
	}
	if _, err := uuid.Parse(claims.OrgID); err != nil {
		return nil, errors.New("access token org_id is not a UUID")
	}
	if _, err := uuid.Parse(claims.SessionID); err != nil {
		return nil, errors.New("access token session_id is not a UUID")
	}
	return claims, nil
}

// NewRefreshToken generates an opaque 256-bit refresh token (base64url) and its
// SHA-256 hash for server-side storage.
func NewRefreshToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashRefreshToken(raw), nil
}

// HashRefreshToken returns the hex SHA-256 of a refresh token. Stored hashes
// mean a database leak is not a credential leak (ADR-007).
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}