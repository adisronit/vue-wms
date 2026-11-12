package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	apihttp "wms-backend/internal/platform/http"
)

// ClaimsContextKey is where RequireJWT stores validated claims for
// downstream handlers to read.
const ClaimsContextKey = "jwt_claims"

// RequireJWT is a reusable, JWT-ready middleware skeleton: it validates a
// Bearer token's signature and expiry against secret and rejects the
// request if either check fails.
//
// It is intentionally NOT wired into any route in this phase — there is
// no users table yet, no login endpoint to issue a token, and nothing to
// authorize against. It exists so the auth module built in a later phase
// has a tested foundation to build on rather than starting from scratch.
func RequireJWT(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			apihttp.Fail(c, http.StatusUnauthorized, "missing or malformed Authorization header", "UNAUTHORIZED")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(header, "Bearer ")

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrTokenSignatureInvalid
			}
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err != nil || !token.Valid {
			apihttp.Fail(c, http.StatusUnauthorized, "invalid or expired token", "UNAUTHORIZED")
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			apihttp.Fail(c, http.StatusUnauthorized, "invalid token claims", "UNAUTHORIZED")
			c.Abort()
			return
		}

		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}
