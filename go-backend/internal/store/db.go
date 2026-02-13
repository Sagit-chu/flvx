// Package store provides a thin dialect-aware wrapper around database/sql,
// enabling transparent use of both SQLite and PostgreSQL.
package store

import (
	"database/sql"
	"strconv"
	"strings"
)

// Dialect identifies the underlying database engine.
type Dialect int

const (
	DialectSQLite Dialect = iota
	DialectPostgres
)

// String returns a human-readable dialect name.
func (d Dialect) String() string {
	switch d {
	case DialectSQLite:
		return "sqlite"
	case DialectPostgres:
		return "postgres"
	default:
		return "unknown"
	}
}

// DB wraps *sql.DB with dialect awareness.
type DB struct {
	raw     *sql.DB
	dialect Dialect
}

// Wrap creates a new dialect-aware DB from an existing *sql.DB.
func Wrap(raw *sql.DB, dialect Dialect) *DB {
	return &DB{raw: raw, dialect: dialect}
}

// Dialect returns the database dialect.
func (db *DB) Dialect() Dialect {
	if db == nil {
		return DialectSQLite
	}
	return db.dialect
}

// RawDB returns the underlying *sql.DB.
func (db *DB) RawDB() *sql.DB {
	if db == nil {
		return nil
	}
	return db.raw
}

// Close closes the underlying connection.
func (db *DB) Close() error {
	if db == nil || db.raw == nil {
		return nil
	}
	return db.raw.Close()
}

// Ping verifies the connection is alive.
func (db *DB) Ping() error {
	return db.raw.Ping()
}

// Exec executes a query with transparent placeholder and syntax rewriting.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.raw.Exec(db.rewrite(query), args...)
}

// Query executes a query that returns rows, with transparent rewriting.
func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.raw.Query(db.rewrite(query), args...)
}

// QueryRow executes a query that returns at most one row, with transparent rewriting.
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.raw.QueryRow(db.rewrite(query), args...)
}

// Begin starts a transaction, returning a dialect-aware Tx.
func (db *DB) Begin() (*Tx, error) {
	tx, err := db.raw.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{raw: tx, dialect: db.dialect}, nil
}

// ExecReturningID executes an INSERT and returns the auto-generated id.
//   - SQLite: uses LastInsertId()
//   - PostgreSQL: appends RETURNING id and uses QueryRow().Scan()
func (db *DB) ExecReturningID(query string, args ...any) (int64, error) {
	q := db.rewrite(query)
	if db.dialect == DialectPostgres {
		q = ensureReturningID(q)
		var id int64
		if err := db.raw.QueryRow(q, args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := db.raw.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Tx wraps *sql.Tx with dialect awareness.
type Tx struct {
	raw     *sql.Tx
	dialect Dialect
}

// Exec executes a query inside the transaction with transparent rewriting.
func (tx *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.raw.Exec(rewriteQuery(tx.dialect, query), args...)
}

// Query executes a query that returns rows inside the transaction.
func (tx *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.raw.Query(rewriteQuery(tx.dialect, query), args...)
}

// QueryRow executes a query that returns at most one row inside the transaction.
func (tx *Tx) QueryRow(query string, args ...any) *sql.Row {
	return tx.raw.QueryRow(rewriteQuery(tx.dialect, query), args...)
}

// Commit commits the transaction.
func (tx *Tx) Commit() error { return tx.raw.Commit() }

// Rollback aborts the transaction.
func (tx *Tx) Rollback() error { return tx.raw.Rollback() }

// ExecReturningID executes an INSERT inside the transaction and returns the id.
func (tx *Tx) ExecReturningID(query string, args ...any) (int64, error) {
	q := rewriteQuery(tx.dialect, query)
	if tx.dialect == DialectPostgres {
		q = ensureReturningID(q)
		var id int64
		if err := tx.raw.QueryRow(q, args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}
	res, err := tx.raw.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) rewrite(query string) string {
	return rewriteQuery(db.dialect, query)
}

func rewriteQuery(dialect Dialect, query string) string {
	if dialect != DialectPostgres {
		return query
	}
	query = rewriteUserIdentifier(query)
	query = rewriteInsertOrIgnore(query)
	query = rewritePlaceholders(query)
	return query
}

func rewriteUserIdentifier(query string) string {
	var buf strings.Builder
	buf.Grow(len(query) + 16)
	i := 0
	for i < len(query) {
		if end, ok := skipSQLProtectedSegment(query, i); ok {
			buf.WriteString(query[i:end])
			i = end
			continue
		}

		ch := query[i]
		if isIdentifierChar(ch) {
			j := i + 1
			for j < len(query) && isIdentifierChar(query[j]) {
				j++
			}
			tok := query[i:j]
			if strings.EqualFold(tok, "user") {
				buf.WriteString(`"user"`)
			} else {
				buf.WriteString(tok)
			}
			i = j
			continue
		}

		buf.WriteByte(ch)
		i++
	}
	return buf.String()
}

func isIdentifierChar(ch byte) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return ch == '_'
}

func rewriteInsertOrIgnore(query string) string {
	start, end, ok := findKeywordSequenceOutside(query, []string{"INSERT", "OR", "IGNORE", "INTO"}, 0)
	if !ok {
		return query
	}

	rewritten := query[:start] + "INSERT INTO" + query[end:]
	rewritten = strings.TrimRight(rewritten, "; \t\n")

	insertIntoEnd := start + len("INSERT INTO")
	if _, _, hasOnConflict := findKeywordSequenceOutside(rewritten, []string{"ON", "CONFLICT"}, insertIntoEnd); hasOnConflict {
		return rewritten
	}

	if retStart, _, hasReturning := findKeywordSequenceOutside(rewritten, []string{"RETURNING"}, insertIntoEnd); hasReturning {
		prefix := strings.TrimRight(rewritten[:retStart], " \t\n")
		suffix := strings.TrimLeft(rewritten[retStart:], " \t\n")
		return prefix + " ON CONFLICT DO NOTHING " + suffix
	}

	return rewritten + " ON CONFLICT DO NOTHING"
}

func rewritePlaceholders(query string) string {
	var buf strings.Builder
	buf.Grow(len(query) + 16)
	n := 1
	for i := 0; i < len(query); i++ {
		if end, ok := skipSQLProtectedSegment(query, i); ok {
			buf.WriteString(query[i:end])
			i = end - 1
			continue
		}

		ch := query[i]
		if ch == '?' {
			buf.WriteByte('$')
			buf.WriteString(strconv.Itoa(n))
			n++
			continue
		}
		buf.WriteByte(ch)
	}
	return buf.String()
}

func ensureReturningID(query string) string {
	trimmed := strings.TrimRight(query, "; \t\n")
	if _, _, ok := findKeywordSequenceOutside(trimmed, []string{"RETURNING"}, 0); ok {
		return trimmed
	}
	return trimmed + " RETURNING id"
}

func findKeywordSequenceOutside(query string, keywords []string, from int) (int, int, bool) {
	if len(keywords) == 0 {
		return 0, 0, false
	}
	if from < 0 {
		from = 0
	}
	if from >= len(query) {
		return 0, 0, false
	}

	matched := 0
	seqStart := -1

	for i := from; i < len(query); {
		if end, ok := skipSQLProtectedSegment(query, i); ok {
			i = end
			continue
		}

		ch := query[i]
		if isIdentifierChar(ch) {
			j := i + 1
			for j < len(query) && isIdentifierChar(query[j]) {
				j++
			}
			tok := query[i:j]

			if strings.EqualFold(tok, keywords[matched]) {
				if matched == 0 {
					seqStart = i
				}
				matched++
				if matched == len(keywords) {
					return seqStart, j, true
				}
			} else if strings.EqualFold(tok, keywords[0]) {
				seqStart = i
				matched = 1
			} else {
				matched = 0
				seqStart = -1
			}

			i = j
			continue
		}

		if !isSQLSpace(ch) {
			matched = 0
			seqStart = -1
		}
		i++
	}

	return 0, 0, false
}

func skipSQLProtectedSegment(query string, i int) (int, bool) {
	if i < 0 || i >= len(query) {
		return 0, false
	}

	switch query[i] {
	case '\'':
		return skipSingleQuotedLiteral(query, i), true
	case '"':
		return skipDoubleQuotedIdentifier(query, i), true
	case '-':
		if i+1 < len(query) && query[i+1] == '-' {
			return skipLineComment(query, i), true
		}
	case '/':
		if i+1 < len(query) && query[i+1] == '*' {
			return skipBlockComment(query, i), true
		}
	case '$':
		if end, ok := skipDollarQuotedLiteral(query, i); ok {
			return end, true
		}
	}

	return 0, false
}

func skipSingleQuotedLiteral(query string, i int) int {
	for j := i + 1; j < len(query); j++ {
		if query[j] != '\'' {
			continue
		}
		if j+1 < len(query) && query[j+1] == '\'' {
			j++
			continue
		}
		return j + 1
	}
	return len(query)
}

func skipDoubleQuotedIdentifier(query string, i int) int {
	for j := i + 1; j < len(query); j++ {
		if query[j] != '"' {
			continue
		}
		if j+1 < len(query) && query[j+1] == '"' {
			j++
			continue
		}
		return j + 1
	}
	return len(query)
}

func skipLineComment(query string, i int) int {
	for j := i + 2; j < len(query); j++ {
		if query[j] == '\n' {
			return j
		}
	}
	return len(query)
}

func skipBlockComment(query string, i int) int {
	depth := 1
	for j := i + 2; j < len(query)-1; j++ {
		if query[j] == '/' && query[j+1] == '*' {
			depth++
			j++
			continue
		}
		if query[j] == '*' && query[j+1] == '/' {
			depth--
			j++
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(query)
}

func skipDollarQuotedLiteral(query string, i int) (int, bool) {
	if i < 0 || i >= len(query) || query[i] != '$' {
		return 0, false
	}

	if i+1 >= len(query) {
		return 0, false
	}

	var endTag int
	if query[i+1] == '$' {
		endTag = i + 1
	} else {
		if !isDollarTagStart(query[i+1]) {
			return 0, false
		}
		j := i + 2
		for j < len(query) && isDollarTagChar(query[j]) {
			j++
		}
		if j >= len(query) || query[j] != '$' {
			return 0, false
		}
		endTag = j
	}

	tag := query[i : endTag+1]
	if closeIdx := strings.Index(query[endTag+1:], tag); closeIdx >= 0 {
		return endTag + 1 + closeIdx + len(tag), true
	}
	return len(query), true
}

func isDollarTagStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDollarTagChar(ch byte) bool {
	if isDollarTagStart(ch) {
		return true
	}
	return ch >= '0' && ch <= '9'
}

func isSQLSpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}
