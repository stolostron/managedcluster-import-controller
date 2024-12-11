package flightctl

import (
	"testing"
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

func TestFlightCtl_tokenCloseToExpire(t *testing.T) {
	now := time.Now()

	testcases := []struct {
		name         string
		expireTime   time.Time
		timeDuration time.Duration
		expected     bool
	}{
		{
			name:         "token not close to expire",
			expireTime:   now.Add(10 * 24 * time.Hour), // expires in 10 days
			timeDuration: 7 * 24 * time.Hour,           // 7 day threshold
			expected:     false,
		},
		{
			name:         "token close to expire",
			expireTime:   now.Add(5 * 24 * time.Hour),  // expires in 5 days
			timeDuration: 7 * 24 * time.Hour,           // 7 day threshold
			expected:     true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := createTestToken(tc.expireTime)
			if err != nil {
				t.Fatalf("failed to create test token: %v", err)
			}

			actual, err := tokenCloseToExpire(token, tc.timeDuration)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if actual != tc.expected {
				t.Errorf("expected: %v, actual: %v", tc.expected, actual)
			}
		})
	}
}
