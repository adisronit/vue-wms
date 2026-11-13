// Package notification is Phase 21's minimal notification inbox — read,
// list, and mark-read for the authenticated user's own rows, plus the
// idempotent create path the stock-adjustment reminder consumer
// (cmd/worker) calls. It is not a general notification platform: the
// only writer today is that one reminder consumer, and the only
// business meaning "type"/"reference_type" ever carry is
// STOCK_ADJUSTMENT_REMINDER / STOCK_ADJUSTMENT. See PROJECT_STATUS.md
// for the recipient-resolution (internal/user's
// FindActiveIDsWithPermission) and idempotency (message_id, per
// recipient) design.
package notification

import "time"

// Type is a closed set of notification kinds. Only one exists today —
// deliberately not left as a free-form string, so a future second type
// is a one-line addition here rather than an undocumented convention.
type Type string

const (
	TypeStockAdjustmentReminder Type = "STOCK_ADJUSTMENT_REMINDER"
)

// Notification is the GORM model for the notifications table
// (migrations/0033). MessageID is the originating RabbitMQ message's own
// message_id — see the migration's own doc for why it is NOT NULL and
// why uniqueness is scoped to (message_id, user_id), not message_id
// alone.
type Notification struct {
	ID int64 `gorm:"column:id;primaryKey"`
	// TenantID (migrations/0045, 0048, Phase 29A) defaults to 1 — see
	// internal/role.Role's own TenantID doc for the full rationale.
	// Defense-in-depth per tenant-er-design.md §3 — UserID remains the
	// primary scoping control for this table.
	TenantID      int64      `gorm:"column:tenant_id;not null;default:1"`
	Type          Type       `gorm:"column:type"`
	UserID        int64      `gorm:"column:user_id"`
	ReferenceType *string    `gorm:"column:reference_type"`
	ReferenceID   *int64     `gorm:"column:reference_id"`
	Title         string     `gorm:"column:title"`
	Message       string     `gorm:"column:message"`
	IsRead        bool       `gorm:"column:is_read"`
	MessageID     string     `gorm:"column:message_id"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
	ReadAt        *time.Time `gorm:"column:read_at"`
}

func (Notification) TableName() string { return "notifications" }
