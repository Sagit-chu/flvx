package contract_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"go-backend/internal/auth"
	httpserver "go-backend/internal/http"
	"go-backend/internal/http/handler"
	"go-backend/internal/store/sqlite"
)

func TestPostgresNodeCreateRepairsMissingIDDefaultContract(t *testing.T) {
	baseDSN := strings.TrimSpace(os.Getenv("FLVX_POSTGRES_TEST_DSN"))
	if baseDSN == "" {
		t.Skip("set FLVX_POSTGRES_TEST_DSN to run postgres contract tests")
	}

	schemaName := "contract_node_id_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("open postgres admin connection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`)
		_ = adminDB.Close()
	})

	if _, err := adminDB.Exec(`CREATE SCHEMA "` + schemaName + `"`); err != nil {
		t.Fatalf("create schema %s: %v", schemaName, err)
	}

	testDSN, err := withSearchPath(baseDSN, schemaName)
	if err != nil {
		t.Fatalf("build schema dsn: %v", err)
	}

	repo, err := sqlite.OpenPostgres(testDSN)
	if err != nil {
		t.Fatalf("open postgres repository: %v", err)
	}
	if _, err := repo.DB().Exec(`ALTER TABLE node ALTER COLUMN id DROP DEFAULT`); err != nil {
		_ = repo.Close()
		t.Fatalf("drop node.id default to simulate drift: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close repository before reopen: %v", err)
	}

	repo, err = sqlite.OpenPostgres(testDSN)
	if err != nil {
		t.Fatalf("reopen postgres repository: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	var columnDefault sql.NullString
	if err := repo.DB().QueryRow(`
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'node'
		  AND column_name = 'id'
		LIMIT 1
	`).Scan(&columnDefault); err != nil {
		t.Fatalf("query node.id default: %v", err)
	}
	if !columnDefault.Valid || !strings.Contains(strings.ToLower(columnDefault.String), "nextval(") {
		t.Fatalf("expected node.id default to be nextval(...), got %q", columnDefault.String)
	}

	jwtSecret := "postgres-contract-secret"
	router := httpserver.NewRouter(handler.New(repo, jwtSecret), jwtSecret)
	token, err := auth.GenerateToken(1, "admin_user", 0, jwtSecret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	body := strings.NewReader(`{"name":"pg-repair-node","serverIp":"10.77.0.10"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/node/create", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assertCode(t, resp, 0)

	var nodeID int64
	if err := repo.DB().QueryRow(`SELECT id FROM node WHERE name = ? ORDER BY id DESC LIMIT 1`, "pg-repair-node").Scan(&nodeID); err != nil {
		t.Fatalf("query created node: %v", err)
	}
	if nodeID <= 0 {
		t.Fatalf("expected positive node id, got %d", nodeID)
	}
}

func withSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
