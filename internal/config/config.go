package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	LogLevel string `env:"LOG_LEVEL" envDefault:"error"`

	Port int `env:"PORT" envDefault:"8080"`

	TimescaleDSN string `env:"TIMESCALE_DSN,required"`

	// TimescaleTable is the schema-qualified hypertable holding the AirGradient
	// readings, as written by airgradient-timescaledb-inserter.
	TimescaleTable string `env:"TIMESCALE_TABLE" envDefault:"sensors.airgradient"`

	// TimescaleAQITable holds the inserter's precomputed AQI rows: a rolling
	// 24 hour index written every scrape.
	TimescaleAQITable string `env:"TIMESCALE_AQI_TABLE" envDefault:"sensors.airgradient_aqi"`

	// SerialNumber limits both tables to one monitor. Leave unset when the
	// inserter only scrapes one.
	SerialNumber string `env:"SERIAL_NUMBER"`

	// QueryTimeout bounds one query against TimescaleDB.
	QueryTimeout time.Duration `env:"QUERY_TIMEOUT" envDefault:"10s"`

	// DragonflyHost enables the response cache for the serve command when set.
	// Neither command requires it: serve runs uncached without it, and the
	// publish command never uses the cache.
	DragonflyHost        string        `env:"DRAGONFLY_HOST"`
	DragonflyPort        int           `env:"DRAGONFLY_PORT" envDefault:"6379"`
	DragonflyAuth        string        `env:"DRAGONFLY_AUTH"`
	DragonflyKeyPrefix   string        `env:"DRAGONFLY_KEY_PREFIX" envDefault:"aqi"`
	CacheResultsDuration time.Duration `env:"CACHE_RESULTS_DURATION" envDefault:"5m"`

	// PublishBucketURL is a gocloud.dev blob URL the publish command writes
	// to, e.g. "gs://my-bucket" or "file:///tmp/out".
	PublishBucketURL string `env:"PUBLISH_BUCKET_URL"`

	// PublishPrefix is the key prefix inside the bucket.
	PublishPrefix string `env:"PUBLISH_PREFIX" envDefault:"v1"`

	// PublishCacheControl is set on every uploaded object. The default keeps
	// readers at most a minute behind while letting a CDN absorb the traffic.
	PublishCacheControl string `env:"PUBLISH_CACHE_CONTROL" envDefault:"public, max-age=60"`

	AuthenticationEnabled bool     `env:"AUTHENTICATION_ENABLED" envDefault:"false"`
	APIKeys               []string `env:"API_KEYS"`

	MetricsEnabled bool `env:"METRICS_ENABLED" envDefault:"true"`
	MetricsPort    int  `env:"METRICS_PORT" envDefault:"8081"`

	Local bool `env:"LOCAL" envDefault:"false"`

	TracingEnabled    bool    `env:"TRACING_ENABLED" envDefault:"false"`
	TracingSampleRate float64 `env:"TRACING_SAMPLERATE" envDefault:"0.01"`
	TracingService    string  `env:"TRACING_SERVICE" envDefault:"aqi-api"`
	TracingVersion    string  `env:"TRACING_VERSION"`
}

func NewConfig() (*Config, error) {
	var cfg Config

	err := env.Parse(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}
