package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"battle-go-api/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return priv, &priv.PublicKey
}

func makeToken(t *testing.T, priv *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(priv)
	require.NoError(t, err)
	return signed
}

// ── Valid token ────────────────────────────────────────────────────────────────

func TestValidateToken_ValidRS256_ReturnsUserID(t *testing.T) {
	priv, pub := generateTestKeyPair(t)
	tokenStr := makeToken(t, priv, jwt.MapClaims{
		"user_id": float64(42),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	userID, err := auth.ValidateToken(tokenStr, pub)

	require.NoError(t, err)
	assert.Equal(t, 42, userID)
}

// ── Expired token ──────────────────────────────────────────────────────────────

func TestValidateToken_ExpiredToken_ReturnsError(t *testing.T) {
	priv, pub := generateTestKeyPair(t)
	tokenStr := makeToken(t, priv, jwt.MapClaims{
		"user_id": float64(42),
		"exp":     time.Now().Add(-time.Hour).Unix(), // expired 1h ago
	})

	_, err := auth.ValidateToken(tokenStr, pub)

	assert.ErrorIs(t, err, auth.ErrExpiredToken)
}

// ── Invalid signature ──────────────────────────────────────────────────────────

func TestValidateToken_WrongPublicKey_ReturnsError(t *testing.T) {
	priv, _ := generateTestKeyPair(t)
	_, wrongPub := generateTestKeyPair(t) // different key pair

	tokenStr := makeToken(t, priv, jwt.MapClaims{
		"user_id": float64(1),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	_, err := auth.ValidateToken(tokenStr, wrongPub)

	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateToken_Tampered_ReturnsError(t *testing.T) {
	priv, pub := generateTestKeyPair(t)
	tokenStr := makeToken(t, priv, jwt.MapClaims{
		"user_id": float64(1),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	tampered := tokenStr[:len(tokenStr)-4] + "xxxx"

	_, err := auth.ValidateToken(tampered, pub)

	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

// ── Missing claims ─────────────────────────────────────────────────────────────

func TestValidateToken_MissingUserID_ReturnsError(t *testing.T) {
	priv, pub := generateTestKeyPair(t)
	tokenStr := makeToken(t, priv, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
		// no user_id
	})

	_, err := auth.ValidateToken(tokenStr, pub)

	assert.ErrorIs(t, err, auth.ErrMissingUserID)
}

func TestValidateToken_UserIDNotNumeric_ReturnsError(t *testing.T) {
	priv, pub := generateTestKeyPair(t)
	tokenStr := makeToken(t, priv, jwt.MapClaims{
		"user_id": "not-a-number",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})

	_, err := auth.ValidateToken(tokenStr, pub)

	assert.ErrorIs(t, err, auth.ErrMissingUserID)
}

// ── Algorithm enforcement ──────────────────────────────────────────────────────

func TestValidateToken_HS256Token_ReturnsError(t *testing.T) {
	// HS256 token should be rejected — only RS256 accepted
	_, pub := generateTestKeyPair(t)

	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": float64(1),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := hmacToken.SignedString([]byte("secret"))

	_, err := auth.ValidateToken(tokenStr, pub)

	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}
