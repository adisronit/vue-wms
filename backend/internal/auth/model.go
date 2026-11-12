package auth

// LoginRequest is the POST /api/v1/auth/login request body.
type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"admin"`
	Password string `json:"password" binding:"required" example:"changeme"`
} //	@name	LoginRequest

// RoleSummary is the minimal role info embedded in auth responses — just
// enough for the frontend to label the current user, not the full role
// record.
type RoleSummary struct {
	Code string `json:"code" example:"ADMIN"`
	Name string `json:"name" example:"Administrator"`
} //	@name	RoleSummary

// UserSummary is the authenticated user's profile as returned by login
// and /auth/me. Password and password hash are never included.
type UserSummary struct {
	ID          int64       `json:"id,string" example:"1"`
	Username    string      `json:"username" example:"admin"`
	FullName    string      `json:"full_name" example:"Administrator"`
	Email       string      `json:"email" example:"admin@smartwms.local"`
	Role        RoleSummary `json:"role"`
	Permissions []string    `json:"permissions"`
} //	@name	UserSummary

// LoginData is the `data` payload of a successful login response.
type LoginData struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type" example:"Bearer"`
	ExpiresIn   int         `json:"expires_in" example:"3600"`
	User        UserSummary `json:"user"`
} //	@name	LoginData
