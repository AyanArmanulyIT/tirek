package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"tirek/backend/internal/platform/migrate"
)

// adminURL is the test database URL used for schema setup and migrations. It
// must be able to create roles, schemas, and tables (the postgres owner role
// in local dev; the CI testcontainer superuser).
func adminURL() string {
	if u := os.Getenv("TIREK_TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://tirek:tirek@localhost:5433/tirek_test?sslmode=disable"
}

// appURL connects as the low-privilege application role so RLS is actually
// exercised end-to-end.
func appURL() string {
	if u := os.Getenv("TIREK_TEST_APP_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://tirek_app:tirek_app@localhost:5433/tirek_test?sslmode=disable"
}

func migrationsDir() string {
	// go test runs with the package directory as CWD (backend/internal/integration),
	// so backend/migrations is two levels up.
	return filepath.Join("..", "..", "migrations")
}

// provision resets the test schema, provisions roles, and applies migrations.
// It returns pools for the admin (migration/owner) role and the app role.
func provision(ctx context.Context) (*pgxpool.Pool, *pgxpool.Pool) {
	admin, err := pgxpool.New(ctx, adminURL())
	if err != nil {
		log.Fatalf("connect admin pool: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		log.Fatalf("ping admin pool: %v", err)
	}

	if _, err := admin.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		log.Fatalf("reset schema: %v", err)
	}
	if err := ensureRole(ctx, admin, "tirek_app", "tirek_app"); err != nil {
		log.Fatalf("ensure tirek_app: %v", err)
	}
	if err := ensureRole(ctx, admin, "tirek_migrator", "tirek_migrator"); err != nil {
		log.Fatalf("ensure tirek_migrator: %v", err)
	}
	if err := migrate.Apply(ctx, admin, migrationsDir()); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	app, err := pgxpool.New(ctx, appURL())
	if err != nil {
		log.Fatalf("connect app pool: %v", err)
	}
	if err := app.Ping(ctx); err != nil {
		log.Fatalf("ping app pool: %v", err)
	}
	return admin, app
}

func ensureRole(ctx context.Context, pool *pgxpool.Pool, name, password string) error {
	var exists bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", name).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	// name/password are fixed constants in this repository.
	_, err := pool.Exec(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD '%s'", name, password))
	return err
}