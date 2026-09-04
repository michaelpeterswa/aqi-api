// Package publish precomputes the API's responses and uploads them to a
// public bucket, so static consumers read a snapshot instead of hitting the
// database.
package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gocloud.dev/blob"

	"github.com/michaelpeterswa/aqi-api/internal/handlers"
	"github.com/michaelpeterswa/aqi-api/internal/timescale"
)

// Reader is the slice of the timescale client the publisher needs, so tests
// can substitute a fake.
type Reader interface {
	ColumnParameters(column string, timeBucket string, lookbackInterval string) timescale.GetColumnTemplateParameters
	ColumnLastParameters(column string) timescale.GetColumnLastTemplateParameters
	AQILastParameters() timescale.GetAQILastTemplateParameters
	AQIWindowParameters(timeBucket string, lookbackInterval string) timescale.GetAQIWindowTemplateParameters
	GetColumn(ctx context.Context, tp timescale.GetColumnTemplateParameters) ([]timescale.GetColumnResponse, error)
	GetColumnLast(ctx context.Context, tp timescale.GetColumnLastTemplateParameters) (*timescale.GetColumnLastResponse, error)
	GetAQILast(ctx context.Context, tp timescale.GetAQILastTemplateParameters) (*timescale.AQILastResponse, error)
	GetAQIWindow(ctx context.Context, tp timescale.GetAQIWindowTemplateParameters) ([]timescale.AQIPointResponse, error)
}

// Publisher computes one snapshot and writes it to the bucket.
type Publisher struct {
	Reader       Reader
	Bucket       *blob.Bucket
	Prefix       string
	CacheControl string
}

// Envelope wraps every per-endpoint object with the time its queries ran, so
// a consumer can tell how fresh the data is without a second request. Data
// holds the same shape the HTTP API serves for that endpoint.
type Envelope struct {
	GeneratedAt time.Time `json:"generated_at"`
	Data        any       `json:"data"`
}

// MetricSnapshot is one raw metric's precomputed responses: the newest
// reading and every window, keyed by window name.
type MetricSnapshot struct {
	Last    *timescale.GetColumnLastResponse         `json:"last,omitempty"`
	Windows map[string][]timescale.GetColumnResponse `json:"windows"`
}

// AQISnapshot is the index's precomputed responses.
type AQISnapshot struct {
	Last    *timescale.AQILastResponse              `json:"last,omitempty"`
	Windows map[string][]timescale.AQIPointResponse `json:"windows"`
}

// Snapshot is the combined document: every metric and the AQI in one object
// so a static site can hydrate a whole dashboard with a single request.
type Snapshot struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	Metrics     map[string]MetricSnapshot `json:"metrics"`
	AQI         AQISnapshot               `json:"aqi"`
}

// Run computes and uploads the snapshot: one object per endpoint
// ({prefix}/{metric}/last.json, {prefix}/{metric}/{window}.json, the same
// under {prefix}/aqi/) plus the combined {prefix}/snapshot.json. Every object
// carries the same generated_at, the time this run's queries started, so
// consumers agree on recency no matter which file they read.
func (p *Publisher) Run(ctx context.Context) error {
	generatedAt := time.Now().UTC()
	snapshot := Snapshot{
		GeneratedAt: generatedAt,
		Metrics:     make(map[string]MetricSnapshot),
		AQI:         AQISnapshot{Windows: make(map[string][]timescale.AQIPointResponse)},
	}

	for _, metric := range handlers.Metrics {
		ms := MetricSnapshot{Windows: make(map[string][]timescale.GetColumnResponse)}

		last, err := p.Reader.GetColumnLast(ctx, p.Reader.ColumnLastParameters(metric.Column))
		if err != nil && !errors.Is(err, timescale.ErrNoData) {
			return fmt.Errorf("get last %s: %w", metric.Name, err)
		}
		ms.Last = last

		if last != nil {
			if err := p.put(ctx, p.key(metric.Name, "last"), Envelope{GeneratedAt: generatedAt, Data: last}); err != nil {
				return err
			}
		}

		for _, window := range handlers.Windows {
			points, err := p.Reader.GetColumn(ctx, p.Reader.ColumnParameters(metric.Column, window.TimeBucket, window.LookbackInterval))
			if err != nil {
				return fmt.Errorf("get %s %s: %w", metric.Name, window.Name, err)
			}
			if points == nil {
				points = []timescale.GetColumnResponse{}
			}
			ms.Windows[window.Name] = points

			if err := p.put(ctx, p.key(metric.Name, window.Name), Envelope{GeneratedAt: generatedAt, Data: points}); err != nil {
				return err
			}
		}

		snapshot.Metrics[metric.Name] = ms
	}

	aqiLast, err := p.Reader.GetAQILast(ctx, p.Reader.AQILastParameters())
	if err != nil && !errors.Is(err, timescale.ErrNoData) {
		return fmt.Errorf("get last aqi: %w", err)
	}
	snapshot.AQI.Last = aqiLast

	if aqiLast != nil {
		if err := p.put(ctx, p.key(handlers.AQIName, "last"), Envelope{GeneratedAt: generatedAt, Data: aqiLast}); err != nil {
			return err
		}
	}

	for _, window := range handlers.Windows {
		points, err := p.Reader.GetAQIWindow(ctx, p.Reader.AQIWindowParameters(window.TimeBucket, window.LookbackInterval))
		if err != nil {
			return fmt.Errorf("get aqi %s: %w", window.Name, err)
		}
		if points == nil {
			points = []timescale.AQIPointResponse{}
		}
		snapshot.AQI.Windows[window.Name] = points

		if err := p.put(ctx, p.key(handlers.AQIName, window.Name), Envelope{GeneratedAt: generatedAt, Data: points}); err != nil {
			return err
		}
	}

	if err := p.put(ctx, fmt.Sprintf("%s/snapshot.json", p.Prefix), snapshot); err != nil {
		return err
	}

	slog.Info("snapshot published",
		slog.Int("metrics", len(snapshot.Metrics)),
		slog.Time("generated_at", snapshot.GeneratedAt))
	return nil
}

// key builds the object key for one endpoint.
func (p *Publisher) key(name string, leaf string) string {
	return fmt.Sprintf("%s/%s/%s.json", p.Prefix, name, leaf)
}

// put marshals and uploads one object. A blob write is only visible once
// complete, so readers see the previous version or the new one, never a
// partial document.
func (p *Publisher) put(ctx context.Context, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", key, err)
	}

	err = p.Bucket.WriteAll(ctx, key, data, &blob.WriterOptions{
		ContentType:  "application/json",
		CacheControl: p.CacheControl,
	})
	if err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	return nil
}
