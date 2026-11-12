package notification

import "time"

// Response is one row of GET /notifications — matches the frontend's
// notification-dropdown shape directly (no separate detail endpoint;
// requirement 8's own "do not create excessive notification CRUD").
type Response struct {
	ID            int64     `json:"id,string" example:"1"`
	Type          Type      `json:"type" example:"STOCK_ADJUSTMENT_REMINDER"`
	ReferenceType *string   `json:"reference_type" example:"STOCK_ADJUSTMENT"`
	ReferenceID   *int64    `json:"reference_id,string"`
	Title         string    `json:"title" example:"Pending Stock Adjustment Reminder"`
	Message       string    `json:"message" example:"ADJ-000123 has been pending for 52h and is overdue."`
	IsRead        bool      `json:"is_read" example:"false"`
	CreatedAt     time.Time `json:"created_at"`
} //	@name	NotificationResponse

// ListParams narrows GET /notifications to the caller's own rows
// (UserID is always set by the handler from the JWT, never a client-
// supplied value) plus the one optional filter the frontend badge/list
// needs. IsRead nil means "any"; non-nil filters to exactly that state.
type ListParams struct {
	UserID int64
	IsRead *bool
	Page   int
	Limit  int
}
