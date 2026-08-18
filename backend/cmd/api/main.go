package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"tirek/backend/internal/httpapi"
	"tirek/backend/internal/platform/config"
	"tirek/backend/internal/platform/db"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
		logger.Fatal().Err(err).Msg("invalid configuration")
	}

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

	api := httpapi.New(cfg, pool, logger)
	srv := &http.Server{
		Addr:              ":" + cfg.API.Port,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info().Str("addr", srv.Addr).Msg("api listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("http server failed")
		}
	}()

	<-ctx.Done()
	logger.Info().Msg("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}