// Package database owns the single GORM connection to SQL Server shared
// by the API server and the worker. It intentionally does not register
// any models yet — Phase 1 is infrastructure only, business tables land
// in later phases.
package database

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"wms-backend/config"
)

// Manager owns the *gorm.DB and tracks live connectivity so the health
// endpoint and background reconnect loop can both reason about it without
// re-dialing on every request.
type Manager struct {
	cfg    config.DatabaseConfig
	logger *slog.Logger

	mu        sync.RWMutex
	db        *gorm.DB
	connected bool
}

// NewManager builds an unconnected Manager. Call Connect to establish the
// initial connection.
func NewManager(cfg config.DatabaseConfig, logger *slog.Logger) *Manager {
	return &Manager{cfg: cfg, logger: logger}
}

// Connect performs a bounded retry/backoff connection attempt. SQL Server
// inside Docker Compose can take tens of seconds to accept connections
// after the container reports "started", so a single failed attempt here
// must not be treated as fatal by the caller — it returns an error instead
// of exiting the process, so main() can decide to keep serving in a
// degraded state while StartReconnectLoop keeps trying in the background.
func (m *Manager) Connect(ctx context.Context, retry config.StartupRetry) error {
	wait := retry.InitialWait

	var lastErr error
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		db, err := m.dial()
		if err == nil {
			m.setConnected(db, true)
			m.logger.Info("database connected", slog.Int("attempt", attempt))
			return nil
		}

		lastErr = err
		m.logger.Warn("database connection attempt failed",
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

	return errors.Join(errors.New("database: exhausted startup retry attempts"), lastErr)
}

func (m *Manager) dial() (*gorm.DB, error) {
	db, err := gorm.Open(sqlserver.Open(m.cfg.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Reasonable pool defaults for a warehouse-scale workload; revisited
	// once real query patterns exist in later phases.
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return db, nil
}

// StartReconnectLoop watches connectivity in the background and keeps
// retrying on a fixed interval whenever the last known state is
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

				db, err := m.dial()
				if err != nil {
					m.logger.Warn("database reconnect attempt failed", slog.String("error", err.Error()))
					continue
				}

				m.setConnected(db, true)
				m.logger.Info("database reconnected")
			}
		}
	}()
}

// Ping reports live connectivity for the health endpoint. It also updates
// the cached connected state, so a dependency that drops between reconnect
// ticks is reflected immediately rather than waiting for the next tick.
func (m *Manager) Ping(ctx context.Context) error {
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()

	if db == nil {
		return errors.New("database: not connected")
	}

	sqlDB, err := db.DB()
	if err != nil {
		m.setConnectedState(false)
		return err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		m.setConnectedState(false)
		return err
	}

	m.setConnectedState(true)
	return nil
}

// DB returns the underlying *gorm.DB. Callers in later phases will use
// this to register models and run queries; nothing in Phase 1 does.
func (m *Manager) DB() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db
}

// IsConnected reports the last known connectivity state without
// performing a network round trip.
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// Close releases the underlying connection pool. Used during graceful
// shutdown.
func (m *Manager) Close() error {
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()

	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (m *Manager) setConnected(db *gorm.DB, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
	m.connected = connected
}

func (m *Manager) setConnectedState(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = connected
}
