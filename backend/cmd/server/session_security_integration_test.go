package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"svoyshef/internal/store"
	"svoyshef/migrations"
)

func TestNFR04_Integration_CredentialIsolationExpiryAndExport(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	ctx := context.Background()
	owner := newItClient()
	itAddToCart(t, owner, srv, itKhinkaliID, "menu")
	credential := owner.credential()
	id := owner.analyticSessionID(t, pool)
	if credential == id {
		t.Fatal("cookie credential must differ from analytical session ID")
	}
	hash := sha256.Sum256([]byte(credential))
	var storedHash []byte
	var created, expires time.Time
	var raw string
	if err := pool.QueryRow(ctx, `SELECT credential_hash, created_at, expires_at, row_to_json(s)::text FROM sessions s WHERE id = $1::uuid`, id).Scan(&storedHash, &created, &expires, &raw); err != nil {
		t.Fatal("session query failed")
	}
	if !bytes.Equal(storedHash, hash[:]) || expires.Sub(created) != 12*time.Hour || strings.Contains(raw, credential) {
		t.Fatal("session must persist only credential hash with fixed absolute expiry")
	}
	t.Setenv("DATABASE_URL", os.Getenv("M2_IT_DATABASE_URL"))
	var csvOut bytes.Buffer
	if err := runExportEvents(nil, &csvOut); err != nil {
		t.Fatal("event export failed")
	}
	if strings.Contains(csvOut.String(), credential) || strings.Contains(csvOut.String(), hex.EncodeToString(hash[:])) {
		t.Fatal("credential or hash leaked into CSV")
	}
	rows, err := csv.NewReader(&csvOut).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][1] != id {
		t.Fatal("CSV must contain only the analytical session ID")
	}
	attacker := newItClient()
	attacker.jar["session"] = rows[1][1]
	if cart := itGetCart(t, attacker, srv); len(cart.Lines) != 0 || attacker.credential() == id {
		t.Fatal("exported analytical ID authenticated access to the owner's cart")
	}
	for _, value := range []string{credential, strings.ToUpper(credential)} {
		owner.mu.Lock()
		owner.jar["session"] = value
		owner.mu.Unlock()
		resp := owner.get(t, srv.URL+"/api/v1/cart?table=99")
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var cart itCartView
		if err != nil || json.Unmarshal(body, &cart) != nil || strings.Contains(string(body), credential) || strings.Contains(string(body), "credential") || strings.Contains(string(body), hex.EncodeToString(hash[:])) {
			t.Fatal("cart JSON must not expose credential or hash")
		}
		if resp.StatusCode != http.StatusOK || len(resp.Cookies()) != 0 || len(cart.Lines) != 1 {
			t.Fatal("actual credential, including uppercase, must reuse the cart without cookie reissue")
		}
	}
	var after time.Time
	if err := pool.QueryRow(ctx, `SELECT expires_at FROM sessions WHERE id = $1::uuid`, id).Scan(&after); err != nil || !after.Equal(expires) {
		t.Fatal("credential reuse must not renew absolute expiry")
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at = now() WHERE id = $1::uuid`, id); err != nil {
		t.Fatal("could not set explicit expiration boundary")
	}
	if authenticatedID, _, err := store.NewSessionStore(pool).GetByCredentialHash(ctx, hash); err != nil || authenticatedID != "" {
		t.Fatal("credential at or beyond its expiration boundary must not authenticate")
	}
	if cart := itGetCart(t, owner, srv); len(cart.Lines) != 0 || strings.EqualFold(owner.credential(), credential) {
		t.Fatal("expired cookie must create a new empty session")
	}
	var preserved int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cart_items WHERE session_id = $1::uuid`, id).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatal("expiry must preserve historical carts")
	}
	var leaked bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events e WHERE row_to_json(e)::text LIKE '%' || $1 || '%' OR row_to_json(e)::text LIKE '%' || $2 || '%')`, credential, hex.EncodeToString(hash[:])).Scan(&leaked); err != nil || leaked {
		t.Fatal("events must not persist credential or hash")
	}
}

func TestNFR04_Integration_LegacyDataPreservedWithoutAuthentication(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	ctx := context.Background()
	var legacyID string
	if err := pool.QueryRow(ctx, `INSERT INTO sessions (id, table_id) VALUES (gen_random_uuid(), 12) RETURNING id::text`).Scan(&legacyID); err != nil {
		t.Fatal("could not seed legacy session")
	}
	if _, err := store.NewCartStore(pool).AddItem(ctx, legacyID, itKhinkaliID, "menu"); err != nil {
		t.Fatal("could not seed legacy cart")
	}
	seedExportEvent(t, pool, legacyID, nil, "page_open", `{}`, time.Now().UTC())
	client := newItClient()
	client.jar["session"] = legacyID
	resp := client.get(t, srv.URL+"/api/v1/cart?table=24")
	var cart itCartView
	decodeBody(t, resp, &cart)
	if resp.StatusCode != http.StatusOK || len(cart.Lines) != 0 || len(resp.Cookies()) != 1 || client.credential() == legacyID {
		t.Fatal("legacy ID must create a new empty anonymous session")
	}
	newID := client.analyticSessionID(t, pool)
	var table int
	if err := pool.QueryRow(ctx, `SELECT table_id FROM sessions WHERE id = $1::uuid`, newID).Scan(&table); err != nil || table != 24 {
		t.Fatal("replacement session must use table from the current request")
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT credential_hash IS NULL AND expires_at IS NULL
		AND (SELECT count(*) FROM cart_items WHERE session_id = s.id) = 1
		AND (SELECT count(*) FROM events WHERE session_id = s.id) = 1
		FROM sessions s WHERE id = $1::uuid`, legacyID).Scan(&preserved); err != nil || !preserved {
		t.Fatal("legacy session, cart and events must be retained without backfill")
	}
}

func TestNFR04_Integration_SessionDBFailureIsClosed(t *testing.T) {
	srv, pool, _ := newIntegrationServer(t)
	client := newItClient()
	resp := client.get(t, srv.URL+"/api/v1/menu")
	_ = resp.Body.Close()
	pool.Close()
	for _, c := range []*itClient{client, newItClient()} {
		resp := c.get(t, srv.URL+"/api/v1/cart")
		var body struct {
			Error struct{ Code string }
		}
		decodeBody(t, resp, &body)
		if resp.StatusCode != http.StatusInternalServerError || body.Error.Code != "internal" || len(resp.Cookies()) != 0 {
			t.Fatal("lookup/create database failure must return generic 500 without new credentials")
		}
	}
}

func TestNFR04_Integration_MigrationPreservesRowsAndEnforcesConstraints(t *testing.T) {
	_, pool, _ := newIntegrationServer(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("begin migration test failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE SCHEMA m5_auth_migration_test; SET LOCAL search_path TO m5_auth_migration_test`); err != nil {
		t.Fatal("isolated migration schema creation failed")
	}
	apply := func(name string) {
		t.Helper()
		script, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, strings.Split(string(script), "-- +goose Down")[0]); err != nil {
			t.Fatal("migration application failed")
		}
	}
	apply("0001_init.sql")
	apply("0002_item_availability.sql")
	var legacyID string
	if err := tx.QueryRow(ctx, `INSERT INTO sessions (id, table_id) VALUES (gen_random_uuid(), 12) RETURNING id::text`).Scan(&legacyID); err != nil {
		t.Fatal("legacy session seed failed")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO carts (session_id) VALUES ($1::uuid)`, legacyID); err != nil {
		t.Fatal("legacy cart seed failed")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO cart_items (session_id, item_id, qty, added_from) VALUES ($1::uuid, 101, 2, 'menu')`, legacyID); err != nil {
		t.Fatal("legacy cart line seed failed")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO events (session_id, table_id, name, payload) VALUES ($1::uuid, 12, 'page_open', '{}'::jsonb)`, legacyID); err != nil {
		t.Fatal("legacy event seed failed")
	}
	var before string
	snapshot := `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(s) - 'credential_hash' - 'expires_at') FROM sessions s),
		(SELECT jsonb_agg(to_jsonb(c)) FROM carts c),
		(SELECT jsonb_agg(to_jsonb(c)) FROM cart_items c), (SELECT jsonb_agg(to_jsonb(e)) FROM events e))::text`
	if err := tx.QueryRow(ctx, snapshot).Scan(&before); err != nil {
		t.Fatal("snapshot before migration failed")
	}
	apply("0003_session_credentials.sql")
	var after string
	if err := tx.QueryRow(ctx, snapshot).Scan(&after); err != nil || before != after {
		t.Fatal("migration changed existing carts or events")
	}
	var legacy bool
	if err := tx.QueryRow(ctx, `SELECT credential_hash IS NULL AND expires_at IS NULL AND table_id = 12 FROM sessions WHERE id = $1::uuid`, legacyID).Scan(&legacy); err != nil || !legacy {
		t.Fatal("migration must preserve the legacy row without credential backfill")
	}
	hash := sha256.Sum256([]byte(rand.Text()))
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (id, credential_hash, expires_at) VALUES (gen_random_uuid(), $1, now() + interval '1 hour')`, hash[:]); err != nil {
		t.Fatal("valid hashed credential rejected")
	}
	for _, tc := range []struct {
		name string
		hash []byte
		exp  *time.Time
		code string
	}{
		{"hash without expiry", make([]byte, 32), nil, "23514"},
		{"expiry without hash", nil, exportDay(2027, 1, 1), "23514"},
		{"short hash", make([]byte, 31), exportDay(2027, 1, 1), "23514"},
		{"long hash", make([]byte, 33), exportDay(2027, 1, 1), "23514"},
		{"empty hash", []byte{}, exportDay(2027, 1, 1), "23514"},
		{"duplicate hash", hash[:], exportDay(2027, 1, 1), "23505"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal("savepoint failed")
			}
			defer func() { _ = child.Rollback(ctx) }()
			_, err = child.Exec(ctx, `INSERT INTO sessions (id, credential_hash, expires_at) VALUES (gen_random_uuid(), $1, $2)`, tc.hash, tc.exp)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatal("session hash/expiry constraint did not reject invalid state")
			}
		})
	}
	// Multiple legacy NULL credentials remain valid; unique hashes do not merge legacy rows.
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (id) VALUES (gen_random_uuid()), (gen_random_uuid())`); err != nil {
		t.Fatal("nullable legacy credential pairs must remain insertable")
	}
}
