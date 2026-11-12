// Package config loads application configuration from environment
// variables. There is no config file support by design: every deployment
// target (local go run, Docker Compose, CI) sets environment variables
// consistently, and CLAUDE.md forbids hardcoded secrets.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the fully-resolved application configuration shared by the
// API server and the worker binary.
type Config struct {
	AppEnv  string
	AppPort string

	DB DatabaseConfig

	JWTSecret    string
	JWTExpiresIn time.Duration

	RabbitMQ RabbitMQConfig

	SwaggerEnabled bool

	// DBTransactionTimeout bounds a durable business mutation's SQL
	// transaction once it has been detached from the inbound request's
	// own cancellable context (see internal/platform/dbctx) — added in
	// Phase 8 after a client disconnecting mid-transaction was found
	// live to orphan an open transaction indefinitely (PROJECT_STATUS.md).
	DBTransactionTimeout time.Duration

	// StockAdjustmentPendingWarningHours is Phase 20's configurable
	// follow-up threshold: a Stock Adjustment still PENDING this many
	// hours after its own updated_at (see internal/stockadjustment's
	// pendingAging doc) is flagged is_overdue=true by the API. Visibility
	// only — nothing in this codebase auto-approves or auto-rejects
	// based on it.
	StockAdjustmentPendingWarningHours int

	// StockAdjustmentReminderIntervalMinutes is Phase 21's scan cadence:
	// how often the server-owned scanner (see cmd/server/main.go) checks
	// for overdue PENDING adjustments and publishes reminder jobs. The
	// same StockAdjustmentPendingWarningHours value doubles as the
	// reminder cooldown (see internal/stockadjustment's
	// ClaimAndListOverdueForReminder) — no separate cooldown knob.
	StockAdjustmentReminderIntervalMinutes int

	// ExportDir is Phase 22's server-side storage path for generated
	// export files (internal/report) — the export_files Docker volume
	// is mounted at this same path in both backend and worker
	// containers (see docker-compose.yml), so a file the worker writes
	// is immediately readable by the server's download endpoint.
	ExportDir string

	// TenantMembershipCacheTTL bounds how long
	// middleware.RequireTenantContext trusts a positive tenant-membership
	// check before re-querying the database — the cache window
	// docs/proposals/wms-saas/tenant-isolation-security.md §3 step 3
	// leaves as an explicit, accepted latency/revocation-visibility
	// tradeoff rather than a silent gap. Negative results are never
	// cached regardless of this value (see that middleware's own doc).
	TenantMembershipCacheTTL time.Duration
}

// DatabaseConfig holds SQL Server connection settings.
type DatabaseConfig struct {
	Host                   string
	Port                   string
	User                   string
	Password               string
	Name                   string
	Encrypt                bool
	TrustServerCertificate bool
}

// RabbitMQConfig holds AMQP connection settings.
type RabbitMQConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	VHost    string
}

// requiredVar describes an environment variable that must be present for
// the application to start.
type requiredVar struct {
	name string
	dest *string
}

// Load reads and validates configuration from the process environment.
// It fails fast (returns an error rather than a partially-populated
// Config) when a mandatory variable is missing, so misconfiguration is
// caught at startup instead of surfacing later as a confusing runtime
// error.
func Load() (*Config, error) {
	loadDotEnv()

	cfg := &Config{}

	required := []requiredVar{
		{"APP_ENV", &cfg.AppEnv},
		{"APP_PORT", &cfg.AppPort},
		{"DB_HOST", &cfg.DB.Host},
		{"DB_PORT", &cfg.DB.Port},
		{"DB_USER", &cfg.DB.User},
		{"DB_PASSWORD", &cfg.DB.Password},
		{"DB_NAME", &cfg.DB.Name},
		{"JWT_SECRET", &cfg.JWTSecret},
		{"RABBITMQ_HOST", &cfg.RabbitMQ.Host},
		{"RABBITMQ_PORT", &cfg.RabbitMQ.Port},
		{"RABBITMQ_USER", &cfg.RabbitMQ.User},
		{"RABBITMQ_PASSWORD", &cfg.RabbitMQ.Password},
	}

	var missing []string

	for _, v := range required {
		val, ok := os.LookupEnv(v.name)
		if !ok || strings.TrimSpace(val) == "" {
			missing = append(missing, v.name)
			continue
		}
		*v.dest = val
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}

	// RABBITMQ_VHOST is allowed to be empty (root vhost "/"), so it is
	// not part of the required list above, but it must still be set
	// explicitly to avoid ambiguity between "unset" and "/".
	vhost, ok := os.LookupEnv("RABBITMQ_VHOST")
	if !ok {
		return nil, fmt.Errorf("missing required environment variable(s): RABBITMQ_VHOST")
	}
	cfg.RabbitMQ.VHost = vhost

	cfg.DB.Encrypt = parseBool(os.Getenv("DB_ENCRYPT"), false)
	cfg.DB.TrustServerCertificate = parseBool(os.Getenv("DB_TRUST_SERVER_CERTIFICATE"), true)
	cfg.SwaggerEnabled = parseBool(os.Getenv("SWAGGER_ENABLED"), true)
	cfg.JWTExpiresIn = parseDuration(os.Getenv("JWT_EXPIRES_IN"), time.Hour)
	cfg.DBTransactionTimeout = parseDuration(os.Getenv("DB_TRANSACTION_TIMEOUT"), 30*time.Second)
	cfg.StockAdjustmentPendingWarningHours = parseInt(os.Getenv("STOCK_ADJUSTMENT_PENDING_WARNING_HOURS"), 48)
	cfg.StockAdjustmentReminderIntervalMinutes = parseInt(os.Getenv("STOCK_ADJUSTMENT_REMINDER_INTERVAL_MINUTES"), 15)
	cfg.ExportDir = os.Getenv("EXPORT_DIR")
	if cfg.ExportDir == "" {
		cfg.ExportDir = "/app/exports"
	}

	cfg.TenantMembershipCacheTTL = parseDuration(os.Getenv("TENANT_MEMBERSHIP_CACHE_TTL"), 60*time.Second)

	return cfg, nil
}

// loadDotEnv populates the process environment from .env files, for local
// development and for running the compiled binaries directly on Windows
// Server (no Docker, no container-supplied environment). It is
// best-effort: a missing .env is not an error, since production
// deployments (Docker containers, or a Windows service with real
// environment variables configured) may not have one at all.
//
// Candidates are tried in priority order — current working directory
// first, then the directory containing the running executable, so
// `wms-api.exe` and `wms-migrate.exe` both find `.env` next to themselves
// even when Windows starts them from another working directory (e.g. a
// scheduled task or a service whose "Start in" directory isn't set).
//
// godotenv.Load never overwrites a variable that already exists in the OS
// environment, so this ordering only ever fills gaps: real Windows/
// container-supplied environment variables always win over .env, and a
// value found in the cwd's .env wins over the same key in the exe
// directory's .env.
func loadDotEnv() {
	var loaded []string
	for _, path := range dotEnvCandidates() {
		if err := godotenv.Load(path); err == nil {
			loaded = append(loaded, path)
		}
	}

	if len(loaded) == 0 {
		fmt.Println("config: .env not found, using system environment variables")
		return
	}
	for _, path := range loaded {
		fmt.Printf("config: loaded .env from %s\n", path)
	}
}

// dotEnvCandidates returns the absolute .env paths to try, in priority
// order, with duplicates removed (Windows paths are compared
// case-insensitively, e.g. when the cwd and the executable's directory
// are the same directory, so that file is never loaded twice).
func dotEnvCandidates() []string {
	var candidates []string
	seen := make(map[string]bool)

	add := func(path string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		key := strings.ToLower(filepath.Clean(abs))
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, abs)
	}

	add(".env")

	if exePath, err := os.Executable(); err == nil {
		add(filepath.Join(filepath.Dir(exePath), ".env"))
	}

	return candidates
}

func parseBool(val string, fallback bool) bool {
	if val == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}
	return parsed
}

// parseInt accepts a plain integer string (e.g. "48"). Like parseBool/
// parseDuration, a missing or malformed value falls back rather than
// failing startup — this configures a follow-up warning threshold, not
// a secret or a value whose absence should be fatal.
func parseInt(val string, fallback int) int {
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return parsed
}

// parseDuration accepts a Go duration string (e.g. "1h", "30m"). It is
// not part of the required-var validation above — a missing or malformed
// JWT_EXPIRES_IN falls back to fallback rather than failing startup,
// since an expiry has a reasonable default and isn't a secret.
func parseDuration(val string, fallback time.Duration) time.Duration {
	if val == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return parsed
}

// DSN builds the SQL Server connection string consumed by
// gorm.io/driver/sqlserver.
func (d DatabaseConfig) DSN() string {
	return d.dsnForDatabase(d.Name)
}

// MasterDSN connects to SQL Server's built-in master database instead of
// the application database. cmd/migrate uses this once, before anything
// else, to create the application database itself on a brand new SQL
// Server instance (a fresh `docker compose up` has no "wms" database
// yet) — every other connection in the app always targets DSN().
func (d DatabaseConfig) MasterDSN() string {
	return d.dsnForDatabase("master")
}

func (d DatabaseConfig) dsnForDatabase(database string) string {
	return fmt.Sprintf(
		"sqlserver://%s:%s@%s:%s?database=%s&encrypt=%s&trustServerCertificate=%s",
		d.User, d.Password, d.Host, d.Port, database,
		boolToSQLServerFlag(d.Encrypt), boolToSQLServerFlag(d.TrustServerCertificate),
	)
}

func boolToSQLServerFlag(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// AMQPURL builds the AMQP connection URL consumed by amqp091-go.
func (r RabbitMQConfig) AMQPURL() string {
	vhost := r.VHost
	if !strings.HasPrefix(vhost, "/") {
		vhost = "/" + vhost
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%s%s", r.User, r.Password, r.Host, r.Port, vhost)
}

// StartupRetry describes the bounded retry/backoff policy used by both
// the database and RabbitMQ connectors during application startup, so a
// dependency that is still booting (common in a fresh `docker compose up`)
// does not immediately crash-loop the container.
type StartupRetry struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
}

// DefaultStartupRetry is used by both platform/database and
// messaging/rabbitmq unless a caller overrides it.
var DefaultStartupRetry = StartupRetry{
	MaxAttempts: 10,
	InitialWait: 1 * time.Second,
	MaxWait:     15 * time.Second,
}
