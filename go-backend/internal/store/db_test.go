package store

import "testing"

func TestRewritePlaceholdersSkipsProtectedSegments(t *testing.T) {
	q := `SELECT ?, '?', "id?", $$body ? $$, $tag$X?$tag$, col -- comment ?
FROM t /* block ? */ WHERE id = ?`
	got := rewritePlaceholders(q)
	want := `SELECT $1, '?', "id?", $$body ? $$, $tag$X?$tag$, col -- comment ?
FROM t /* block ? */ WHERE id = $2`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewriteInsertOrIgnoreBasic(t *testing.T) {
	q := `INSERT OR IGNORE INTO user_group_user(user_group_id, user_id, created_time) VALUES(?, ?, ?)`
	got := rewriteInsertOrIgnore(q)
	want := `INSERT INTO user_group_user(user_group_id, user_id, created_time) VALUES(?, ?, ?) ON CONFLICT DO NOTHING`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewriteInsertOrIgnoreBeforeReturning(t *testing.T) {
	q := `INSERT OR IGNORE INTO x(a) VALUES(?) RETURNING id`
	got := rewriteInsertOrIgnore(q)
	want := `INSERT INTO x(a) VALUES(?) ON CONFLICT DO NOTHING RETURNING id`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewriteInsertOrIgnoreNotDuplicatingOnConflict(t *testing.T) {
	q := `INSERT OR IGNORE INTO x(a) VALUES(?) ON CONFLICT(a) DO UPDATE SET a=excluded.a`
	got := rewriteInsertOrIgnore(q)
	want := `INSERT INTO x(a) VALUES(?) ON CONFLICT(a) DO UPDATE SET a=excluded.a`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestEnsureReturningID(t *testing.T) {
	if got := ensureReturningID(`INSERT INTO x(a) VALUES($1)`); got != `INSERT INTO x(a) VALUES($1) RETURNING id` {
		t.Fatalf("missing RETURNING append: %s", got)
	}
	if got := ensureReturningID(`INSERT INTO x(a) VALUES($1) RETURNING other_id`); got != `INSERT INTO x(a) VALUES($1) RETURNING other_id` {
		t.Fatalf("RETURNING should not be duplicated: %s", got)
	}
}

func TestRewriteUserIdentifierSafety(t *testing.T) {
	q := `SELECT user, user_id, 'user', "user", note FROM user -- user
WHERE owner='user'`
	got := rewriteUserIdentifier(q)
	want := `SELECT "user", user_id, 'user', "user", note FROM "user" -- user
WHERE owner='user'`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewriteQueryPostgresPipeline(t *testing.T) {
	q := `INSERT OR IGNORE INTO user(name, note) VALUES(?, '?')`
	got := rewriteQuery(DialectPostgres, q)
	want := `INSERT INTO "user"(name, note) VALUES($1, '?') ON CONFLICT DO NOTHING`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewriteInsertOrIgnoreSkipsStringLiteral(t *testing.T) {
	q := `SELECT 'INSERT OR IGNORE INTO t(a) VALUES(?)' AS q`
	got := rewriteInsertOrIgnore(q)
	if got != q {
		t.Fatalf("string literal should stay unchanged\nwant: %s\ngot:  %s", q, got)
	}
}

func TestRewriteInsertOrIgnoreSkipsCommentedKeyword(t *testing.T) {
	q := `-- INSERT OR IGNORE INTO ignored(a) VALUES(?)
INSERT OR IGNORE INTO real_t(a) VALUES(?)`
	got := rewriteInsertOrIgnore(q)
	want := `-- INSERT OR IGNORE INTO ignored(a) VALUES(?)
INSERT INTO real_t(a) VALUES(?) ON CONFLICT DO NOTHING`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewritePlaceholdersSkipsNestedBlockComment(t *testing.T) {
	q := `SELECT ? /* outer ? /* inner ? */ still_outer ? */ FROM t WHERE id = ?`
	got := rewritePlaceholders(q)
	want := `SELECT $1 /* outer ? /* inner ? */ still_outer ? */ FROM t WHERE id = $2`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewritePlaceholdersSkipsUnterminatedBlockComment(t *testing.T) {
	q := `SELECT ? /* unterminated ? comment`
	got := rewritePlaceholders(q)
	want := `SELECT $1 /* unterminated ? comment`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRewriteUserIdentifierSkipsDollarQuotedAndComment(t *testing.T) {
	q := `SELECT user, $$user ?$$ AS body, col FROM user /* user */ -- user`
	got := rewriteUserIdentifier(q)
	want := `SELECT "user", $$user ?$$ AS body, col FROM "user" /* user */ -- user`
	if got != want {
		t.Fatalf("unexpected rewrite\nwant: %s\ngot:  %s", want, got)
	}
}
