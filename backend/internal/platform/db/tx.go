package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetTenant configures the RLS session settings for a transaction. It must be
// called before any tenant-scoped query runs on the transaction. RLS policies
// compare org_id against app.tenant_id; failing to set it makes tenant-scoped
// reads return no rows (fail closed, see docs/architecture/database.md §3).
func SetTenant(ctx context.Context, tx pgx.Tx, orgID, role string) error {
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", orgID); err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.role', $1, true)", role); err != nil {
		return fmt.Errorf("set role context: %w", err)
	}
	return nil
}

// WithTx runs fn inside a transaction with the tenant context configured for
// orgID. The transaction is committed on success and rolled back on error.
func WithTx(ctx context.Context, pool *pgxpool.Pool, orgID, role string, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := SetTenant(ctx, tx, orgID, role); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// IsUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return asPgError(err, &pgErr) && pgErr.Code == "23505"
}

func asPgError(err error, target **pgconn.PgError) bool {
	for err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			*target = pgErr
			return true
		}
		unw, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unw.Unwrap()
	}
	return false
}