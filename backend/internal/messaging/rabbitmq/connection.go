// Package rabbitmq provides the reusable AMQP connection, publisher,
// consumer and topology foundation shared by the API server (publisher
// side) and the worker (consumer side). No business queues are declared
// here — see topology.go for the single development/infrastructure queue
// used to verify this phase end-to-end.
package rabbitmq

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"wms-backend/config"
)

// Manager owns the single AMQP connection shared by everything that needs
// to publish or consume, and tracks live connectivity for the health
// endpoint and the reconnect loop.
type Manager struct {
	cfg    config.RabbitMQConfig
	logger *slog.Logger

	mu        sync.RWMutex
	conn      *amqp.Connection
	connected bool
}

// NewManager builds an unconnected Manager. Call Connect to establish the
// initial connection.
func NewManager(cfg config.RabbitMQConfig, logger *slog.Logger) *Manager {
	return &Manager{cfg: cfg, logger: logger}
}

// Connect performs a bounded retry/backoff connection attempt, mirroring
// database.Manager.Connect: RabbitMQ may still be starting when the API
// server or worker container comes up, so a single failed dial must not
// be fatal here.
func (m *Manager) Connect(ctx context.Context, retry config.StartupRetry) error {
	wait := retry.InitialWait

	var lastErr error
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := amqp.Dial(m.cfg.AMQPURL())
		if err == nil {
			m.setConn(conn)
			m.watchClose(conn)
			m.logger.Info("rabbitmq connected", slog.Int("attempt", attempt))
			return nil
		}

		lastErr = err
		m.logger.Warn("rabbitmq connection attempt failed",
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", retry.MaxAttempts),
			slog.String("error", err.Error()),
			slog.Duration("retry_in", wait),
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}

		wait *= 2
		if wait > retry.MaxWait {
			wait = retry.MaxWait
		}
	}

	return errors.Join(errors.New("rabbitmq: exhausted startup retry attempts"), lastErr)
}

// watchClose listens for an unexpected connection closure (network blip,
// broker restart) and flips the cached connected state so the health
// endpoint and reconnect loop notice immediately instead of on the next
// poll interval.
func (m *Manager) watchClose(conn *amqp.Connection) {
	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
	go func() {
		err, ok := <-closeCh
		if !ok {
			return
		}
		m.logger.Warn("rabbitmq connection closed unexpectedly", slog.String("error", err.Error()))
		m.setConnectedState(false)
	}()
}

// StartReconnectLoop watches connectivity in the background and attempts
// a single reconnect on a fixed interval whenever the last known state is
// disconnected. It returns immediately; cancel ctx to stop it.
func (m *Manager) StartReconnectLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if m.IsConnected() {
					continue
				}

				conn, err := amqp.Dial(m.cfg.AMQPURL())
				if err != nil {
					m.logger.Warn("rabbitmq reconnect attempt failed", slog.String("error", err.Error()))
					continue
				}

				m.setConn(conn)
				m.watchClose(conn)
				m.logger.Info("rabbitmq reconnected")
			}
		}
	}()
}

// OpenChannel opens a fresh AMQP channel on the current connection.
// Channels are cheap and not safe for concurrent use, so publishers and
// consumers each open their own rather than sharing one.
func (m *Manager) OpenChannel() (*amqp.Channel, error) {
	m.mu.RLock()
	conn := m.conn
	connected := m.connected
	m.mu.RUnlock()

	if conn == nil || !connected {
		return nil, errors.New("rabbitmq: not connected")
	}

	return conn.Channel()
}

// IsConnected reports the last known connectivity state without
// performing a network round trip.
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// Close closes the underlying connection. Used during graceful shutdown.
func (m *Manager) Close() error {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil || conn.IsClosed() {
		return nil
	}
	return conn.Close()
}

func (m *Manager) setConn(conn *amqp.Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conn = conn
	m.connected = true
}

func (m *Manager) setConnectedState(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = connected
}
