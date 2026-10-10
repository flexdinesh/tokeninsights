// Package testdb creates isolated disposable PostgreSQL databases for real tests.
// Production packages must never import it.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func New(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("TOKENINSIGHTS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("live PostgreSQL required: run pnpm test or set TOKENINSIGHTS_TEST_POSTGRES_DSN")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid PostgreSQL test configuration")
	}
	admin := stdlib.OpenDB(*config)
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "ti_contract_" + hex.EncodeToString(random[:])
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name+" TEMPLATE template0 LC_COLLATE 'C' LC_CTYPE 'C'"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer func() { _ = admin.Close() }()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		u.RawPath = ""
		q := u.Query()
		q.Del("dbname")
		q.Del("database")
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn + " dbname=" + name
}

func Open(t testing.TB, dsn string) *sql.DB {
	t.Helper()
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
