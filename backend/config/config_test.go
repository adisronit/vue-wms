package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearEnv(t *testing.T, keys []string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

var allRequiredKeys = []string{
	"APP_ENV", "APP_PORT",
	"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME",
	"JWT_SECRET",
	"RABBITMQ_HOST", "RABBITMQ_PORT", "RABBITMQ_USER", "RABBITMQ_PASSWORD", "RABBITMQ_VHOST",
}

func setAllRequired(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"APP_ENV":           "development",
		"APP_PORT":          "8080",
		"DB_HOST":           "sqlserver",
		"DB_PORT":           "1433",
		"DB_USER":           "sa",
		"DB_PASSWORD":       "Sup3rSecret!",
		"DB_NAME":           "wms",
		"JWT_SECRET":        "test-secret",
		"RABBITMQ_HOST":     "rabbitmq",
		"RABBITMQ_PORT":     "5672",
		"RABBITMQ_USER":     "wms",
		"RABBITMQ_PASSWORD": "wms-password",
		"RABBITMQ_VHOST":    "/",
	}
	for k, v := range values {
		t.Setenv(k, v)
	}
}

func TestLoad_MissingRequiredVariable(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)

	// Remove exactly one required variable and confirm Load reports it by
	// name rather than failing silently or panicking.
	t.Setenv("DB_PASSWORD", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error when DB_PASSWORD is missing, got nil")
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Fatalf("expected error to mention DB_PASSWORD, got: %v", err)
	}
}

func TestLoad_AllVariablesSet(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("DB_ENCRYPT", "true")
	t.Setenv("DB_TRUST_SERVER_CERTIFICATE", "false")
	t.Setenv("SWAGGER_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", cfg.AppPort)
	}
	if !cfg.DB.Encrypt {
		t.Error("DB.Encrypt = false, want true")
	}
	if cfg.DB.TrustServerCertificate {
		t.Error("DB.TrustServerCertificate = true, want false")
	}
	if cfg.SwaggerEnabled {
		t.Error("SwaggerEnabled = true, want false")
	}
}

func TestLoad_BooleanDefaults(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	// DB_ENCRYPT, DB_TRUST_SERVER_CERTIFICATE, SWAGGER_ENABLED left unset.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.DB.Encrypt {
		t.Error("DB.Encrypt default = true, want false")
	}
	if !cfg.DB.TrustServerCertificate {
		t.Error("DB.TrustServerCertificate default = false, want true")
	}
	if !cfg.SwaggerEnabled {
		t.Error("SwaggerEnabled default = false, want true")
	}
}

func TestLoad_DBTransactionTimeoutDefault(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	// DB_TRANSACTION_TIMEOUT left unset.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DBTransactionTimeout != 30*time.Second {
		t.Errorf("DBTransactionTimeout default = %v, want 30s", cfg.DBTransactionTimeout)
	}
}

func TestLoad_DBTransactionTimeoutOverride(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("DB_TRANSACTION_TIMEOUT", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DBTransactionTimeout != 45*time.Second {
		t.Errorf("DBTransactionTimeout = %v, want 45s", cfg.DBTransactionTimeout)
	}
}

func TestLoad_StockAdjustmentPendingWarningHoursDefault(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	// STOCK_ADJUSTMENT_PENDING_WARNING_HOURS left unset.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.StockAdjustmentPendingWarningHours != 48 {
		t.Errorf("StockAdjustmentPendingWarningHours default = %v, want 48", cfg.StockAdjustmentPendingWarningHours)
	}
}

func TestLoad_StockAdjustmentPendingWarningHoursOverride(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("STOCK_ADJUSTMENT_PENDING_WARNING_HOURS", "12")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.StockAdjustmentPendingWarningHours != 12 {
		t.Errorf("StockAdjustmentPendingWarningHours = %v, want 12", cfg.StockAdjustmentPendingWarningHours)
	}
}

func TestLoad_StockAdjustmentPendingWarningHoursInvalidFallsBack(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("STOCK_ADJUSTMENT_PENDING_WARNING_HOURS", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.StockAdjustmentPendingWarningHours != 48 {
		t.Errorf("StockAdjustmentPendingWarningHours = %v, want fallback 48", cfg.StockAdjustmentPendingWarningHours)
	}
}

func TestLoad_StockAdjustmentReminderIntervalMinutesDefault(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	// STOCK_ADJUSTMENT_REMINDER_INTERVAL_MINUTES left unset.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.StockAdjustmentReminderIntervalMinutes != 15 {
		t.Errorf("StockAdjustmentReminderIntervalMinutes default = %v, want 15", cfg.StockAdjustmentReminderIntervalMinutes)
	}
}

func TestLoad_StockAdjustmentReminderIntervalMinutesOverride(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("STOCK_ADJUSTMENT_REMINDER_INTERVAL_MINUTES", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.StockAdjustmentReminderIntervalMinutes != 5 {
		t.Errorf("StockAdjustmentReminderIntervalMinutes = %v, want 5", cfg.StockAdjustmentReminderIntervalMinutes)
	}
}

func TestLoad_TenantMembershipCacheTTLDefault(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	// TENANT_MEMBERSHIP_CACHE_TTL left unset.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.TenantMembershipCacheTTL != 60*time.Second {
		t.Errorf("TenantMembershipCacheTTL default = %v, want 60s", cfg.TenantMembershipCacheTTL)
	}
}

func TestLoad_TenantMembershipCacheTTLOverride(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("TENANT_MEMBERSHIP_CACHE_TTL", "30s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.TenantMembershipCacheTTL != 30*time.Second {
		t.Errorf("TenantMembershipCacheTTL = %v, want 30s", cfg.TenantMembershipCacheTTL)
	}
}

func TestLoad_TenantMembershipCacheTTLInvalidFallsBack(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("TENANT_MEMBERSHIP_CACHE_TTL", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.TenantMembershipCacheTTL != 60*time.Second {
		t.Errorf("TenantMembershipCacheTTL fallback = %v, want 60s", cfg.TenantMembershipCacheTTL)
	}
}

func TestLoad_ExportDirDefault(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	// EXPORT_DIR left unset.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.ExportDir != "/app/exports" {
		t.Errorf("ExportDir default = %q, want /app/exports", cfg.ExportDir)
	}
}

func TestLoad_ExportDirOverride(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	setAllRequired(t)
	t.Setenv("EXPORT_DIR", "/tmp/wms-exports")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.ExportDir != "/tmp/wms-exports" {
		t.Errorf("ExportDir = %q, want /tmp/wms-exports", cfg.ExportDir)
	}
}

func TestDotEnvCandidates_CWDFirstAndDeduped(t *testing.T) {
	candidates := dotEnvCandidates()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error: %v", err)
	}
	wantFirst := filepath.Join(cwd, ".env")

	if len(candidates) == 0 {
		t.Fatal("expected at least one .env candidate (the cwd), got none")
	}
	if !strings.EqualFold(candidates[0], wantFirst) {
		t.Errorf("candidates[0] = %q, want %q (cwd .env tried first)", candidates[0], wantFirst)
	}

	// No duplicate paths, even when the test binary's own directory
	// happens to equal the cwd (mirrors the case where a Windows .exe is
	// launched with its own folder as the working directory).
	seen := make(map[string]bool)
	for _, c := range candidates {
		key := strings.ToLower(filepath.Clean(c))
		if seen[key] {
			t.Errorf("dotEnvCandidates() contains duplicate path %q", c)
		}
		seen[key] = true
	}
}

func TestLoad_DotEnvFillsUnsetVariables(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	chdir(t, t.TempDir())

	writeEnvFile(t, ".env", map[string]string{
		"APP_ENV":           "production",
		"APP_PORT":          "9090",
		"DB_HOST":           "sql.internal",
		"DB_PORT":           "1433",
		"DB_USER":           "sa",
		"DB_PASSWORD":       "Sup3rSecret!",
		"DB_NAME":           "wms",
		"JWT_SECRET":        "from-dotenv",
		"RABBITMQ_HOST":     "rabbitmq.internal",
		"RABBITMQ_PORT":     "5672",
		"RABBITMQ_USER":     "wms",
		"RABBITMQ_PASSWORD": "wms-password",
		"RABBITMQ_VHOST":    "/",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected .env alone to satisfy required config, got error: %v", err)
	}
	if cfg.DB.Host != "sql.internal" {
		t.Errorf("DB.Host = %q, want value loaded from .env", cfg.DB.Host)
	}
	if cfg.AppPort != "9090" {
		t.Errorf("AppPort = %q, want value loaded from .env", cfg.AppPort)
	}
}

func TestLoad_SystemEnvVariableTakesPrecedenceOverDotEnv(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	chdir(t, t.TempDir())

	values := map[string]string{
		"APP_ENV":           "production",
		"APP_PORT":          "9090",
		"DB_HOST":           "from-dotenv",
		"DB_PORT":           "1433",
		"DB_USER":           "sa",
		"DB_PASSWORD":       "Sup3rSecret!",
		"DB_NAME":           "wms",
		"JWT_SECRET":        "from-dotenv",
		"RABBITMQ_HOST":     "rabbitmq.internal",
		"RABBITMQ_PORT":     "5672",
		"RABBITMQ_USER":     "wms",
		"RABBITMQ_PASSWORD": "wms-password",
		"RABBITMQ_VHOST":    "/",
	}
	writeEnvFile(t, ".env", values)

	// A real Windows environment variable for DB_HOST must win over the
	// same key in .env.
	t.Setenv("DB_HOST", "from-windows-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.DB.Host != "from-windows-env" {
		t.Errorf("DB.Host = %q, want %q (system env must win over .env)", cfg.DB.Host, "from-windows-env")
	}
}

func TestLoad_NoDotEnvUsesSystemEnvironment(t *testing.T) {
	clearEnv(t, allRequiredKeys)
	chdir(t, t.TempDir())
	setAllRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error when .env is absent but system env is fully configured, got: %v", err)
	}
	if cfg.AppPort != "8080" {
		t.Errorf("AppPort = %q, want 8080", cfg.AppPort)
	}
}

// chdir switches the process working directory to dir for the duration of
// the test and restores the original on cleanup. A stand-in for
// testing.T.Chdir (Go 1.24+) since this module targets go 1.23.
func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%q) error: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
}

func writeEnvFile(t *testing.T, name string, values map[string]string) {
	t.Helper()
	var sb strings.Builder
	for k, v := range values {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	if err := os.WriteFile(name, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	d := DatabaseConfig{
		Host: "sqlserver", Port: "1433", User: "sa", Password: "p@ss",
		Name: "wms", Encrypt: false, TrustServerCertificate: true,
	}

	dsn := d.DSN()
	for _, want := range []string{"sqlserver://sa:p@ss@sqlserver:1433", "database=wms", "encrypt=false", "trustServerCertificate=true"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN() = %q, want it to contain %q", dsn, want)
		}
	}
}

func TestRabbitMQConfig_AMQPURL(t *testing.T) {
	cases := []struct {
		name  string
		vhost string
		want  string
	}{
		{"root vhost", "/", "amqp://wms:secret@rabbitmq:5672/"},
		{"vhost without leading slash gets one", "wms-vhost", "amqp://wms:secret@rabbitmq:5672/wms-vhost"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := RabbitMQConfig{Host: "rabbitmq", Port: "5672", User: "wms", Password: "secret", VHost: tc.vhost}
			if got := r.AMQPURL(); got != tc.want {
				t.Errorf("AMQPURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
