package user

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wms-backend/internal/platform/database/sqlerr"
)

// ErrNotFound is returned by repository lookups that find no matching row.
var ErrNotFound = errors.New("user: not found")

// ErrDuplicate is returned when a unique constraint (username or email)
// would be violated.
var ErrDuplicate = errors.New("user: username or email already exists")

// ListFilter narrows GET /api/v1/users to what the User Management
// screen's search box and role/status <select> filters actually send —
// no sort parameter exists in that screen today, so none is accepted
// here.
type ListFilter struct {
	Search string // matched against full_name, username, email (LIKE)
	RoleID int64  // 0 means "any role"
	Status Status // "" means "any status"
	Page   int
	Limit  int
}

// Repository is a thin GORM wrapper — it holds no business rules
// (uniqueness messaging, password hashing, permission checks all live in
// Service) so it stays trivially testable against a real or sqlite-mocked
// DB if ever needed.
type Repository struct {
	db *gorm.DB
}

// NewRepository builds a Repository bound to the shared *gorm.DB.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, u *User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).First(&u, "username = ?", username).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByUsernameOrEmail resolves identifier against username OR email
// with an exact match only (never LIKE/partial) — the lookup behind
// WMS-29E's "link an existing user to another tenant" flow, which must
// not become a general user-search facility (CLAUDE.md: "Do not expose
// unrestricted global user search to Tenant Admins"). A caller already
// knows the exact identifier (shared out of band); this answers "does a
// user with exactly this identifier exist," nothing broader.
func (r *Repository) FindByUsernameOrEmail(ctx context.Context, identifier string) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).First(&u, "username = ? OR email = ?", identifier, identifier).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) Update(ctx context.Context, u *User) error {
	err := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"username":      u.Username,
		"password_hash": u.PasswordHash,
		"full_name":     u.FullName,
		"email":         u.Email,
		"role_id":       u.RoleID,
		"warehouse_id":  u.WarehouseID,
		"status":        u.Status,
	}).Error
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

// UpdateLastLogin sets last_login_at to the database's own current UTC
// time for id — called only by internal/auth's Service.Login, only
// after credentials have already been verified and the account
// confirmed ACTIVE. Uses SQL Server's SYSUTCDATETIME() rather than a
// Go-computed time.Now(), the same server-authoritative-clock
// convention every created_at/updated_at default in this schema already
// uses, so there is no app/DB clock-skew risk. Never called for a
// failed login, a token refresh, or any other authenticated request.
func (r *Repository) UpdateLastLogin(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Exec(
		"UPDATE users SET last_login_at = SYSUTCDATETIME() WHERE id = ?", id,
	).Error
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&User{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns the page of users matching filter plus the total row
// count (ignoring pagination) for the response's meta.total.
func (r *Repository) List(ctx context.Context, filter ListFilter) ([]User, int64, error) {
	query := r.db.WithContext(ctx).Model(&User{})

	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		query = query.Where("full_name LIKE ? OR username LIKE ? OR email LIKE ?", like, like, like)
	}
	if filter.RoleID != 0 {
		query = query.Where("role_id = ?", filter.RoleID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []User
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Order("full_name ASC").Offset(offset).Limit(filter.Limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// isUniqueViolation delegates to the centralized, error-number-based
// classifier (internal/platform/database/sqlerr). Originally
// implemented here via message substring-matching specifically to
// avoid importing the mssql driver's error type directly — Phase 8
// verified errors.As correctly unwraps it through GORM's own error
// wrapping (see sqlerr's tests), and centralizing sidesteps the class
// of bug Phase 6 hit live (a PRIMARY KEY violation's real message
// matches neither "2627" nor "unique" — see sqlerr's doc).
func isUniqueViolation(err error) bool {
	return sqlerr.IsUniqueViolation(err)
}

// FindActiveIDsWithPermission returns the IDs of every ACTIVE user whose
// role currently grants permissionCode — Phase 21's recipient-resolution
// query for the stock-adjustment reminder consumer (internal/notification),
// computed dynamically from role_permissions/permissions rather than any
// hardcoded role list, so it stays correct if a role's grants ever
// change (e.g. via PUT /roles/{id}/permissions) without this query
// needing an edit. INACTIVE/LOCKED users are excluded — a disabled
// account should not accumulate reminders nobody will read.
func (r *Repository) FindActiveIDsWithPermission(ctx context.Context, permissionCode string) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT u.id
		FROM users u
		JOIN role_permissions rp ON rp.role_id = u.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE p.code = ? AND u.status = 'ACTIVE'
	`, permissionCode).Scan(&ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}
