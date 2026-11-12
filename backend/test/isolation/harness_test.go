// Package isolation is the Phase 29 tenant-isolation security test
// suite (WMS-29). It proves, against a real SQL Server database and the
// real HTTP handler/service/repository stack, which cross-tenant access
// risks identified by docs/proposals/wms-saas/phase27-audit-report.md
// are still open today.
//
// Scope: the Phase 29 module set per
// docs/proposals/wms-saas/saas-module-roadmap.md's "Implementation
// Phases" table — auth, user, role, rbac, warehouse, zone, location,
// product. Every other module (inventory, receiving, putaway, picking,
// shipping, order, stockadjustment, stockcount, replenishment,
// notification, report, dashboard) is Phase 30-32's isolation
// enforcement scope, not this one; see backend/test/isolation/README.md
// for how those modules' repository-level risk is instead covered by
// the static audit in internal/platform/tenantaudit.
//
// IMPORTANT — this suite intentionally fails today. There is no
// tenant_id column on any business table yet (migrations 0043-0046 are
// future Phase 29 migrations not yet written — see migration-plan.md)
// and no tenant claim in the JWT (tenant-isolation-security.md's
// redesigned Claims shape is not yet implemented). Every test below
// therefore documents the CURRENT, UNSAFE behavior as a FAILURE: a test
// fails exactly when today's code lets Tenant A reach Tenant B's data,
// which is always, by the audit's own headline finding. These tests are
// the acceptance criteria Phase 29's real implementation must satisfy —
// not a regression suite over code that exists yet. See README.md for
// the full failure inventory and how to re-run this suite once Phase 29
// lands tenant_id + RequireTenantContext.
package isolation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"wms-backend/internal/platform/testsupport"
)

const testJWTSecret = "isolation-suite-test-secret-not-for-production"

// harness wires one shared router + DB connection for a test file's
// subtests. Built fresh per top-level test via newHarness, never shared
// across top-level tests, so one test's fixtures can never bleed into
// another's — each top-level test gets its own set of users/warehouses/
// products, distinguished only by random-suffixed codes (see fixtures.go)
// since there is no real tenant boundary in the schema to isolate on yet.
type harness struct {
	t      *testing.T
	db     *gorm.DB
	deps   testsupport.RouterDeps
	server *httptest.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db := testsupport.ConnectDB(t)

	deps := testsupport.BuildTestRouter(db, testJWTSecret)
	srv := httptest.NewServer(deps.Engine)
	t.Cleanup(srv.Close)

	return &harness{t: t, db: db, deps: deps, server: srv}
}

// apiResponse is the generic shape every envelope in response.go
// deserializes into — fields beyond Success/Message/Error are read via
// Data as a map, since every ID round-trips as a JSON string
// (`json:"id,string"` throughout the DTOs) and callers that need a
// typed field decode Data a second time with json.Unmarshal.
type apiResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
	Meta    json.RawMessage `json:"meta"`
}

// doRequest issues method/path against h's server, optionally with a
// bearer token and a JSON body, and decodes the standard envelope.
func (h *harness) doRequest(method, path, bearer string, body any) (int, apiResponse) {
	h.t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		h.t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("do request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	var parsed apiResponse
	if resp.ContentLength != 0 {
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil && resp.StatusCode != http.StatusNoContent {
			h.t.Fatalf("decode response body for %s %s: %v", method, path, err)
		}
	}

	return resp.StatusCode, parsed
}

// login authenticates username/password through the real
// POST /api/v1/auth/login endpoint and returns the issued bearer token —
// deliberately going through HTTP rather than calling auth.IssueToken
// directly, so every isolation test below exercises the exact same
// token-issuance path production traffic does.
func (h *harness) login(username, password string) string {
	h.t.Helper()

	status, resp := h.doRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	if status != http.StatusOK {
		h.t.Fatalf("login(%q) failed: status=%d message=%q", username, status, resp.Message)
	}

	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		h.t.Fatalf("login(%q): decode token: %v", username, err)
	}
	if data.AccessToken == "" {
		h.t.Fatalf("login(%q): empty access token", username)
	}
	return data.AccessToken
}

// hashPassword is the one piece of production logic (bcrypt) the
// fixture builders reuse directly, matching exactly how
// cmd/migrate's seedAdminUser and internal/user's Service.Create hash a
// password — never a weaker stand-in, since RequireJWT in these tests
// must authenticate exactly the way a real login does.
func hashPassword(t *testing.T, plain string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(hash)
}

func ctxWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// itoa formats an int64 ID for embedding in a URL path.
func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

// unmarshalData decodes an apiResponse.Data payload into out — a thin
// wrapper so call sites read as "decode the envelope's data" rather than
// a bare json.Unmarshal with no context about what raw is.
func unmarshalData(raw json.RawMessage, out any) error {
	return json.Unmarshal(raw, out)
}

var suffixCounter atomic.Uint32

// uniqueSuffix gives fixture codes/usernames/emails a collision-free
// suffix across subtests sharing one disposable database. Kept
// deliberately short (<=8 base36 chars) — warehouses.code and
// zones.code are VARCHAR(20) (migrations/0005, 0006) and fixture codes
// are built as "WH-<label>-<suffix>"/"Z-<label>-<suffix>", so a suffix
// as long as a raw UnixNano timestamp would truncate and collide on
// insert. Unix-second resolution alone isn't unique across the many
// fixtures one test seeds within the same second, hence the atomic
// counter alongside it.
func uniqueSuffix() string {
	n := suffixCounter.Add(1)
	return strconv.FormatInt(time.Now().Unix(), 36) + strconv.FormatUint(uint64(n%1296), 36)
}
