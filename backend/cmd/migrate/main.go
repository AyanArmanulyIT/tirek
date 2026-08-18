package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"tirek/backend/internal/platform/config"
	"tirek/backend/internal/platform/migrate"
)

// cmd/migrate applies SQL migrations from backend/migrations in order,
// guarded by an advisory lock (safe under concurrent deploys).
// Applied versions are recorded in schema_migrations.
func main() {
	cfg := config.Load()
	if len(os.Args) != 2 {
		log.Fatal("usage: migrate <migrations-dir>")
	}
	dir := os.Args[1]

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	if err := migrate.Apply(ctx, pool, dir); err != nil {
		log.Fatal(err)
	}
	log.Println("migrations up to date")
}