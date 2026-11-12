package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	apihttp "wms-backend/internal/platform/http"
)

// RequirePermission is the second stage of the JWT Middleware -> Permission
// Middleware -> Handler chain CLAUDE.md's RBAC section describes. It must
// run after RequireJWT (it reads the claims RequireJWT already validated
// and stored under ClaimsContextKey) and rejects the request with 403
// unless the token's "permissions" claim contains code.
//
// Permissions are embedded in the JWT at login time (see internal/auth)
// rather than re-queried from SQL Server on every request, to keep
// authorization checks cheap and side-effect free. The tradeoff — a
// permission or role change only takes effect the next time the affected
// user logs in — is accepted for this phase and noted in
// PROJECT_STATUS.md.
func RequirePermission(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := c.Get(ClaimsContextKey)
		if !ok {
			apihttp.Fail(c, http.StatusUnauthorized, "missing authentication", "UNAUTHORIZED")
			c.Abort()
			return
		}

		claims, ok := raw.(jwt.MapClaims)
		if !ok || !hasPermission(claims, code) {
			apihttp.Fail(c, http.StatusForbidden, "permission denied: "+code, "FORBIDDEN")
			c.Abort()
			return
		}

		c.Next()
	}
}

func hasPermission(claims jwt.MapClaims, code string) bool {
	raw, ok := claims["permissions"]
	if !ok {
		return false
	}

	list, ok := raw.([]any)
	if !ok {
		return false
	}

	for _, v := range list {
		if s, ok := v.(string); ok && s == code {
			return true
		}
	}
	return false
}
