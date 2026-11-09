package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNFR04_DatabaseConfigFailuresOmitCredentials(t *testing.T) {
	credential := rand.Text()
	// Invalid port rejects the DSN during parsing, without contacting any DB.
	dsn := "postgres://user:" + credential + "@127.0.0.1:invalid/db"
	pool, err := newPool(context.Background(), dsn)
	if pool != nil {
		pool.Close()
		t.Fatal("malformed DSN must not create a pool")
	}
	if err == nil || err.Error() != "operation=connect_database code=internal" {
		t.Fatal("connection parsing errors must be sanitized")
	}
	if err := runMigrations(context.Background(), dsn); err == nil || err.Error() != "operation=migrate_database code=internal" {
		t.Fatal("migration connection parsing errors must be sanitized")
	}
}

func TestNFR04_MigrationMakefileUsesEnvWithoutDSNInArgvOrOutput(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "goose-stub.sh")
	invoked := filepath.Join(dir, "invoked")
	script := `#!/bin/sh
set -eu
: > "$MIGRATION_TEST_INVOKED"
[ "$GOOSE_DRIVER" = postgres ]
[ "$GOOSE_DBSTRING" = "$MIGRATION_TEST_DSN" ]
[ "$#" = 5 ]
[ "$1" = -env ] && [ "$2" = none ]
[ "$3" = -dir ] && [ "$4" = migrations ]
[ "$5" = "$MIGRATION_TEST_DIRECTION" ]
if [ "${MIGRATION_TEST_FAIL:-}" = 1 ]; then
  printf '%s' "$GOOSE_DBSTRING"
  printf '%s' "$GOOSE_DBSTRING" >&2
  exit 1
fi
`
	if err := os.WriteFile(stub, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	// Generated sentinel: not a usable connection string and never connects to a DB.
	dsn := rand.Text() + " ' \" $ ; & " + rand.Text()
	t.Setenv("DATABASE_URL", dsn)
	t.Setenv("MIGRATION_TEST_DSN", dsn)
	t.Setenv("MIGRATION_TEST_INVOKED", invoked)
	t.Setenv("MIGRATION_TEST_FAIL", "")
	t.Setenv("GOOSE_DBSTRING", "")
	t.Setenv("GOOSE_DRIVER", "")
	t.Setenv("MAKEFLAGS", "")
	t.Setenv("MFLAGS", "")
	for _, direction := range []string{"up", "down"} {
		t.Run(direction, func(t *testing.T) {
			t.Setenv("MIGRATION_TEST_DIRECTION", direction)
			for _, dryRun := range []bool{true, false} {
				args := []string{"--no-print-directory", "-C", "../..", "GOOSE=/bin/sh \"" + stub + "\"", "migrate-" + direction}
				if dryRun {
					args = append(args, "-n")
				}
				output, err := exec.Command("make", args...).CombinedOutput()
				if err != nil {
					t.Fatal("make migration with stub must succeed")
				}
				if strings.Contains(string(output), dsn) || (!dryRun && len(output) != 0) {
					t.Fatal("make must not echo the DSN or expanded recipe")
				}
				if _, err := os.Stat(invoked); dryRun {
					if !os.IsNotExist(err) {
						t.Fatal("dry run must not execute goose")
					}
				} else {
					if err != nil {
						t.Fatal("stub goose was not invoked")
					}
					if err := os.Remove(invoked); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Setenv("MIGRATION_TEST_FAIL", "1")
			output, err := exec.Command("make", "--no-print-directory", "-C", "../..", "GOOSE=/bin/sh \""+stub+"\"", "migrate-"+direction).CombinedOutput()
			if err == nil || strings.Contains(string(output), dsn) || !strings.Contains(string(output), "operation=migrate_"+direction+" code=internal") {
				t.Fatal("tool errors must not expose raw DSN-bearing output")
			}
			if err := os.Remove(invoked); err != nil {
				t.Fatal("failing stub was not invoked")
			}
			t.Setenv("DATABASE_URL", "")
			output, err = exec.Command("make", "--no-print-directory", "-C", "../..", "GOOSE=/bin/sh \""+stub+"\"", "migrate-"+direction).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "DATABASE_URL is required") {
				t.Fatal("missing DATABASE_URL must fail with a safe diagnostic")
			}
			if _, err := os.Stat(invoked); !os.IsNotExist(err) {
				t.Fatal("missing DATABASE_URL must fail before invoking goose")
			}
		})
	}
}
