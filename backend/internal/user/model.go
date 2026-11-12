// Package user owns the users table: the account an operator logs in
// with, its bcrypt password hash, and its single role assignment. It is
// deliberately separate from package auth (login/token issuance) and
// package role (roles/permissions catalog) per CLAUDE.md's per-domain
// package layout.
package user

import "time"

// Status is the account's lifecycle state. Three values, not a boolean,
// because the User Management screen already distinguishes an
// administratively disabled account (INACTIVE) from one locked out after
// too many failed attempts or similar (LOCKED) — a plain is_active flag
// would collapse that distinction.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
	StatusLocked   Status = "LOCKED"
)

// IsValid reports whether s is one of the three statuses the users table
// CHECK constraint (CK_users_status) allows.
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusInactive, StatusLocked:
		return true
	default:
		return false
	}
}

// User is the GORM model for the users table (migrations/0004,
// extended by migrations/0039 with warehouse_id and last_login_at).
type User struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	Username     string     `gorm:"column:username"`
	PasswordHash string     `gorm:"column:password_hash"`
	FullName     string     `gorm:"column:full_name"`
	Email        string     `gorm:"column:email"`
	RoleID       int64      `gorm:"column:role_id"`
	WarehouseID  *int64     `gorm:"column:warehouse_id"`
	Status       Status     `gorm:"column:status"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

// TableName pins the table name explicitly rather than relying on GORM's
// pluralization inference.
func (User) TableName() string { return "users" }
