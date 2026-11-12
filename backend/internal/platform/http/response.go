// Package http holds the Gin engine wiring and the response envelope
// helpers every handler (present and future) uses, so response
// formatting per CLAUDE.md's standard is written exactly once.
package http

import "github.com/gin-gonic/gin"

// SuccessResponse is the envelope for a single-resource success reply.
type SuccessResponse struct {
	Success bool   `json:"success" example:"true"`
	Data    any    `json:"data"`
	Message string `json:"message,omitempty"`
}

// ListMeta carries pagination metadata for list responses.
type ListMeta struct {
	Page       int `json:"page" example:"1"`
	Limit      int `json:"limit" example:"20"`
	Total      int `json:"total" example:"100"`
	TotalPages int `json:"total_pages" example:"5"`
}

// ListResponse is the envelope for a paginated collection reply. No
// handler uses it yet in this phase, but every future list endpoint
// (Products, Inventory, Receiving, ...) will, so the shape is fixed now.
type ListResponse struct {
	Success bool     `json:"success" example:"true"`
	Data    any      `json:"data"`
	Meta    ListMeta `json:"meta"`
}

// ErrorResponse is the envelope for a failure reply.
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Message string `json:"message" example:"error description"`
	Error   string `json:"error" example:"ERROR_CODE"`
}

// OK writes a 200 success envelope.
func OK(c *gin.Context, data any, message string) {
	c.JSON(200, SuccessResponse{Success: true, Data: data, Message: message})
}

// Status writes a success envelope with a caller-chosen status code, for
// cases like a degraded health check that must still report structured
// data.
func Status(c *gin.Context, code int, data any, message string) {
	c.JSON(code, SuccessResponse{Success: true, Data: data, Message: message})
}

// List writes a 200 paginated collection envelope.
func List(c *gin.Context, data any, meta ListMeta) {
	c.JSON(200, ListResponse{Success: true, Data: data, Meta: meta})
}

// Fail writes an error envelope with the given HTTP status code.
func Fail(c *gin.Context, code int, message, errorCode string) {
	c.JSON(code, ErrorResponse{Success: false, Message: message, Error: errorCode})
}
