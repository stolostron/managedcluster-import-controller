package flightctl

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// Helper function to create test tokens
func createTestToken(expirationTime time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(expirationTime),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte("test-secret"))
}
