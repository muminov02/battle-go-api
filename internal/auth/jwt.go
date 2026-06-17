package auth

import (
	"crypto/rsa"
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")
var ErrExpiredToken = errors.New("token expired")
var ErrMissingUserID = errors.New("missing user_id claim")

// ValidateToken parses and validates an RS256 JWT, returns the student user_id.
// Only RS256 algorithm is accepted. Returns typed sentinel errors.
func ValidateToken(tokenStr string, publicKey *rsa.PublicKey) (userID int, err error) {
	token, parseErr := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, ErrInvalidToken
		}
		return publicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))

	if parseErr != nil {
		if errors.Is(parseErr, jwt.ErrTokenExpired) {
			return 0, ErrExpiredToken
		}
		return 0, ErrInvalidToken
	}

	if !token.Valid {
		return 0, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, ErrInvalidToken
	}

	raw, exists := claims["user_id"]
	if !exists {
		return 0, ErrMissingUserID
	}

	f, ok := raw.(float64)
	if !ok {
		return 0, ErrMissingUserID
	}

	return int(f), nil
}
