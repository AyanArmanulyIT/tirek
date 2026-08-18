package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env        string
	LogLevel   string
	API        APIConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	Auth       AuthConfig
	Payments   PaymentsConfig
	Financing  FinancingConfig
	Worker     WorkerConfig
	Observability ObservabilityConfig
}

type APIConfig struct {
	Port         string
	ExternalURL  string
	WebOrigin    string
	WebhookSecret string
}

type DatabaseConfig struct {
	URL     string
	PoolMax int
}

type RedisConfig struct {
	URL string
}

type AuthConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	CookieDomain  string
	CookieSecure  bool
	CookieName    string
}

type PaymentsConfig struct {
	Provider string // mock | kaspi
	Mode     string // sandbox | live
}

type FinancingConfig struct {
	Provider string // mock | partner
}

type WorkerConfig struct {
	OutboxPollInterval time.Duration
	OutboxBatchSize    int
}

type ObservabilityConfig struct {
	OTLPEndpoint string
	SentryDSN    string
}

func Load() Config {
	return Config{
		Env:      get("APP_ENV", "local"),
		LogLevel: get("LOG_LEVEL", "info"),
		API: APIConfig{
			Port:          get("API_PORT", "8080"),
			ExternalURL:   get("API_EXTERNAL_URL", "http://localhost:8080"),
			WebOrigin:     get("WEB_ORIGIN", "http://localhost:3000"),
			WebhookSecret: get("WEBHOOK_SIGNING_SECRET", ""),
		},
		Database: DatabaseConfig{
			URL:     get("DATABASE_URL", "postgres://tirek:tirek@localhost:5432/tirek?sslmode=disable"),
			PoolMax: getInt("DATABASE_POOL_MAX", 20),
		},
		Redis: RedisConfig{URL: get("REDIS_URL", "redis://localhost:6379/0")},
		Auth: AuthConfig{
			AccessSecret:  get("JWT_ACCESS_SECRET", ""),
			RefreshSecret: get("JWT_REFRESH_SECRET", ""),
			AccessTTL:     getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getDuration("JWT_REFRESH_TTL", 720*time.Hour),
			CookieDomain:  get("AUTH_COOKIE_DOMAIN", "localhost"),
			CookieSecure:  get("AUTH_COOKIE_SECURE", "false") == "true",
			CookieName:    get("AUTH_COOKIE_NAME", "tirek_refresh"),
		},
		Payments: PaymentsConfig{
			Provider: get("PAYMENT_PROVIDER", "mock"),
			Mode:     get("PAYMENT_MODE", "sandbox"),
		},
		Financing: FinancingConfig{Provider: get("FINANCING_PROVIDER", "mock")},
		Worker: WorkerConfig{
			OutboxPollInterval: getDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
			OutboxBatchSize:    getInt("OUTBOX_BATCH_SIZE", 100),
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint: get("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			SentryDSN:    get("SENTRY_DSN", ""),
		},
	}
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmtSscan(v, &n); err != nil {
		return def
	}
	return n
}

func getDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func fmtSscan(s string, n *int) (int, error) {
	i := 0
	neg := false
	digits := 0
	for _, r := range strings.TrimSpace(s) {
		if r == '-' && digits == 0 && i == 0 {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			return 0, errParse
		}
		i = i*10 + int(r-'0')
		digits++
	}
	if digits == 0 {
		return 0, errParse
	}
	if neg {
		i = -i
	}
	*n = i
	return 1, nil
}

var errParse = &parseError{}

type parseError struct{}

func (*parseError) Error() string { return "invalid integer" }

// Validate returns an error if required configuration is missing or insecure.
func (c Config) Validate() error {
	if len(c.Auth.AccessSecret) < 32 {
		return errors.New("JWT_ACCESS_SECRET must be at least 32 bytes (generate with: openssl rand -base64 48)")
	}
	if len(c.Auth.RefreshSecret) < 32 {
		return errors.New("JWT_REFRESH_SECRET must be at least 32 bytes (generate with: openssl rand -base64 48)")
	}
	if c.Database.URL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.Auth.CookieName == "" {
		return errors.New("AUTH_COOKIE_NAME must not be empty")
	}
	return nil
}