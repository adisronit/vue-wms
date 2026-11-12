package auth

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer identifies this application as the JWT's issuer.
const Issuer = "smart-wms"

// Claims is the JWT payload issued at login. It intentionally excludes
// password, email, and any other sensitive field — only what's needed to
// identify the caller and authorize a request without a database round
// trip (see middleware.RequirePermission).
type Claims struct {
	Username    string   `json:"username"`
	RoleCode    string   `json:"role"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// IssueToken signs an HS256 JWT for userID, valid for expiresIn from now.
func IssueToken(secret string, expiresIn time.Duration, userID int64, username, roleCode string, permissions []string) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)

	claims := Claims{
		Username:    username,
		RoleCode:    roleCode,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    Issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	return signed, expiresAt, err
}
