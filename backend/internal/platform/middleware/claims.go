package middleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// CurrentUserID reads the "sub" claim RequireJWT stored under
// ClaimsContextKey and parses it back into the numeric user ID. Moved
// here in Phase 6 (originally a private helper inside internal/auth)
// once internal/receiving became a second caller needing the same
// lookup — a shared platform-level helper instead of two copies of the
// same claim-parsing logic.
func CurrentUserID(c *gin.Context) (int64, bool) {
	raw, ok := c.Get(ClaimsContextKey)
	if !ok {
		return 0, false
	}

	claims, ok := raw.(jwt.MapClaims)
	if !ok {
		return 0, false
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return 0, false
	}

	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, false
	}

	return id, true
}
