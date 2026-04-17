package contract_test

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"go-backend/internal/http/handler"
	"go-backend/internal/store/repo"
)

func init() {
	handler.DisableSafeRemoteAddrCheckForTesting = true
}

func mustLastInsertID(t *testing.T, r *repo.Repository, label string) int64 {
	t.Helper()
	var id int64
	if err := r.DB().Raw("SELECT last_insert_rowid()").Row().Scan(&id); err != nil {
		t.Fatalf("read last_insert_rowid for %s: %v", label, err)
	}
	if id <= 0 {
		t.Fatalf("invalid last_insert_rowid for %s: %d", label, id)
	}
	return id
}

func mustQueryInt(t *testing.T, r *repo.Repository, query string, args ...interface{}) int {
	t.Helper()
	var v int
	if err := r.DB().Raw(query, args...).Row().Scan(&v); err != nil {
		t.Fatalf("query int failed: %v (query=%q)", err, query)
	}
	return v
}

func mustQueryInt64(t *testing.T, r *repo.Repository, query string, args ...interface{}) int64 {
	t.Helper()
	var v int64
	if err := r.DB().Raw(query, args...).Row().Scan(&v); err != nil {
		t.Fatalf("query int64 failed: %v (query=%q)", err, query)
	}
	return v
}

func mustQueryString(t *testing.T, r *repo.Repository, query string, args ...interface{}) string {
	t.Helper()
	var v string
	if err := r.DB().Raw(query, args...).Row().Scan(&v); err != nil {
		t.Fatalf("query string failed: %v (query=%q)", err, query)
	}
	return v
}

func mustQueryInt64Int(t *testing.T, r *repo.Repository, query string, args ...interface{}) (int64, int) {
	t.Helper()
	var a int64
	var b int
	if err := r.DB().Raw(query, args...).Row().Scan(&a, &b); err != nil {
		t.Fatalf("query int64+int failed: %v (query=%q)", err, query)
	}
	return a, b
}

func tryQueryString(t *testing.T, r *repo.Repository, query string, args ...interface{}) (string, error) {
	t.Helper()
	var v string
	err := r.DB().Raw(query, args...).Row().Scan(&v)
	if err != nil {
		return "", err
	}
	return v, nil
}

func mustQueryNullString(t *testing.T, r *repo.Repository, query string, args ...interface{}) sql.NullString {
	t.Helper()
	var v sql.NullString
	if err := r.DB().Raw(query, args...).Row().Scan(&v); err != nil {
		t.Fatalf("query null string failed: %v (query=%q)", err, query)
	}
	return v
}

func mustQueryTwoNullStrings(t *testing.T, r *repo.Repository, query string, args ...interface{}) (sql.NullString, sql.NullString) {
	t.Helper()
	var a sql.NullString
	var b sql.NullString
	if err := r.DB().Raw(query, args...).Row().Scan(&a, &b); err != nil {
		t.Fatalf("query two null strings failed: %v (query=%q)", err, query)
	}
	return a, b
}

func mustQueryNodePorts(t *testing.T, r *repo.Repository, query string, args ...interface{}) map[int64]int {
	t.Helper()
	rows, err := r.DB().Raw(query, args...).Rows()
	if err != nil {
		t.Fatalf("query node ports failed: %v (query=%q)", err, query)
	}
	defer rows.Close()

	out := make(map[int64]int)
	for rows.Next() {
		var nodeID int64
		var port int
		if err := rows.Scan(&nodeID, &port); err != nil {
			t.Fatalf("scan node ports row failed: %v (query=%q)", err, query)
		}
		out[nodeID] = port
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate node ports rows failed: %v (query=%q)", err, query)
	}
	return out
}

func tryQueryInt(t *testing.T, r *repo.Repository, query string, args ...interface{}) (int, error) {
	t.Helper()
	var v int
	err := r.DB().Raw(query, args...).Row().Scan(&v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func valueAsInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func valueAsString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func valueAsBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case int:
		return b != 0
	case int64:
		return b != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(b))
		return s == "1" || s == "t" || s == "true" || s == "yes" || s == "y"
	default:
		return false
	}
}

func jsonInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
