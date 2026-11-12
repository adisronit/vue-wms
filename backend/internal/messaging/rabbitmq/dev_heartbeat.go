package rabbitmq

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DevHeartbeat is a DEVELOPMENT / INFRASTRUCTURE-ONLY message shape used
// to verify the publish → exchange → queue → consume pipeline end to end
// in this phase, before any real business message exists. It mirrors the
// message envelope CLAUDE.md recommends for every future async message
// (message_id, event_type, created_at, correlation_id, payload), so it
// exercises the same shape a real consumer will parse later.
type DevHeartbeat struct {
	MessageID     string    `json:"message_id"`
	EventType     string    `json:"event_type"`
	CreatedAt     time.Time `json:"created_at"`
	CorrelationID string    `json:"correlation_id"`
	Payload       struct {
		Source string `json:"source"`
	} `json:"payload"`
}

// NewDevHeartbeat builds a heartbeat tagged with the given source
// (typically the process name, e.g. "server").
func NewDevHeartbeat(source string) DevHeartbeat {
	h := DevHeartbeat{
		MessageID:     uuid.NewString(),
		EventType:     "system.health",
		CreatedAt:     time.Now().UTC(),
		CorrelationID: uuid.NewString(),
	}
	h.Payload.Source = source
	return h
}

// Marshal encodes the heartbeat as the JSON body a Publisher sends.
func (h DevHeartbeat) Marshal() ([]byte, error) {
	return json.Marshal(h)
}
