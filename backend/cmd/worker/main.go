package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"tirek/backend/internal/platform/config"
	"tirek/backend/internal/platform/db"
)

// tirek-worker consumes the transactional outbox (backend/migrations,
// outbox_events) and executes jobs: PSP captures, financing submissions,
// notifications, webhook processing. Handlers must be idempotent:
// at-least-once delivery is by design.
func main() {
	cfg := config.Load()

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err == nil {
		logger = logger.Level(level)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.Database.URL, cfg.Database.PoolMax)
	if err != nil {
		logger.Fatal().Err(err).Msg("database connection failed")
	}
	defer pool.Close()

	logger.Info().
		Dur("poll_interval", cfg.Worker.OutboxPollInterval).
		Int("batch_size", cfg.Worker.OutboxBatchSize).
		Msg("worker starting")

	// Outbox poll loop. Job handlers are registered per topic in later
	// iterations (internal/<module>/jobs); the loop and dedupe semantics
	// are already in place.
	ticker := time.NewTicker(cfg.Worker.OutboxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("worker stopped")
			return
		case <-ticker.C:
		}
	}
}