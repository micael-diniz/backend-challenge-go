//go:build integration

package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

const testDatabaseURLEnvironment = "TEST_DATABASE_URL"

func TestWalletMigrationUpAndDown(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	resetPublicSchema(t, ctx, connection)

	applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
	if !tableExists(t, ctx, connection, "wallets") {
		t.Error("wallets table does not exist after up migration")
	}

	applyMigrationFile(t, ctx, connection, "000001_wallets.down.sql")
	if tableExists(t, ctx, connection, "wallets") {
		t.Error("wallets table still exists after down migration")
	}
}

func TestWalletConstraints(t *testing.T) {
	ctx, connection := openTestDatabase(t)

	tests := []struct {
		name       string
		seed       string
		invalidSQL string
	}{
		{
			name: "duplicate wallet ID",
			seed: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000102',
				'USD', 100, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "duplicate player and currency",
			seed: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000002',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 100, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "negative balance",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', -1, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "nonpositive version",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 0, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "unsupported currency",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'EUR', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "updated before creation",
			invalidSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:01Z', '2026-10-14T12:00:00Z'
			)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPublicSchema(t, ctx, connection)
			applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
			if tt.seed != "" {
				if _, err := connection.Exec(ctx, tt.seed); err != nil {
					t.Fatalf("seed valid wallet: %v", err)
				}
			}
			if _, err := connection.Exec(ctx, tt.invalidSQL); err == nil {
				t.Fatal("invalid wallet insert succeeded")
			}
		})
	}
}

func TestWalletAcceptsValidStates(t *testing.T) {
	ctx, connection := openTestDatabase(t)
	tests := []struct {
		name     string
		validSQL string
	}{
		{
			name: "zero BRL opening state",
			validSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000001',
				'00000000-0000-0000-0000-000000000101',
				'BRL', 0, 1, '2026-10-14T12:00:00Z', '2026-10-14T12:00:00Z'
			)`,
		},
		{
			name: "positive USD transitioned state",
			validSQL: `INSERT INTO wallets (
				id, player_id, currency, balance_minor, version, created_at, updated_at
			) VALUES (
				'00000000-0000-0000-0000-000000000002',
				'00000000-0000-0000-0000-000000000102',
				'USD', 9223372036854775807, 2,
				'2026-10-14T12:00:00Z', '2026-10-14T12:00:01Z'
			)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPublicSchema(t, ctx, connection)
			applyMigrationFile(t, ctx, connection, "000001_wallets.up.sql")
			if _, err := connection.Exec(ctx, tt.validSQL); err != nil {
				t.Fatalf("valid wallet insert failed: %v", err)
			}
		})
	}
}

func openTestDatabase(t *testing.T) (context.Context, *pgx.Conn) {
	t.Helper()
	databaseURL := os.Getenv(testDatabaseURLEnvironment)
	if databaseURL == "" {
		t.Fatalf("%s is required for integration tests", testDatabaseURLEnvironment)
	}

	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(ctx); err != nil {
			t.Errorf("close PostgreSQL connection: %v", err)
		}
	})
	return ctx, connection
}

func resetPublicSchema(t *testing.T, ctx context.Context, connection *pgx.Conn) {
	t.Helper()
	if _, err := connection.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatalf("reset public schema: %v", err)
	}
}

func applyMigrationFile(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
	name string,
) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "migrations", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %q: %v", name, err)
	}
	if _, err := connection.Exec(ctx, string(contents)); err != nil {
		t.Fatalf("apply migration %q: %v", name, err)
	}
}

func tableExists(
	t *testing.T,
	ctx context.Context,
	connection *pgx.Conn,
	name string,
) bool {
	t.Helper()
	var exists bool
	if err := connection.QueryRow(
		ctx,
		"SELECT to_regclass($1) IS NOT NULL",
		"public."+name,
	).Scan(&exists); err != nil {
		t.Fatalf("check table %q: %v", name, err)
	}
	return exists
}
