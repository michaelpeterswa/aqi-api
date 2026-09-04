package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gocloud.dev/blob/memblob"

	"github.com/michaelpeterswa/aqi-api/internal/aqi"
	"github.com/michaelpeterswa/aqi-api/internal/handlers"
	"github.com/michaelpeterswa/aqi-api/internal/timescale"
)

type fakeReader struct {
	// noLast makes the last-value queries report no data, the way an empty
	// table would.
	noLast bool
}

func (f *fakeReader) ColumnParameters(column, timeBucket, lookbackInterval string) timescale.GetColumnTemplateParameters {
	return timescale.GetColumnTemplateParameters{Table: "sensors.airgradient", Column: column, TimeBucket: timeBucket, LookbackInterval: lookbackInterval}
}

func (f *fakeReader) ColumnLastParameters(column string) timescale.GetColumnLastTemplateParameters {
	return timescale.GetColumnLastTemplateParameters{Table: "sensors.airgradient", Column: column}
}

func (f *fakeReader) AQILastParameters() timescale.GetAQILastTemplateParameters {
	return timescale.GetAQILastTemplateParameters{AQITable: "sensors.airgradient_aqi"}
}

func (f *fakeReader) AQIWindowParameters(timeBucket, lookbackInterval string) timescale.GetAQIWindowTemplateParameters {
	return timescale.GetAQIWindowTemplateParameters{AQITable: "sensors.airgradient_aqi", TimeBucket: timeBucket, LookbackInterval: lookbackInterval}
}

func (f *fakeReader) GetColumn(_ context.Context, tp timescale.GetColumnTemplateParameters) ([]timescale.GetColumnResponse, error) {
	return []timescale.GetColumnResponse{
		{Time: time.Unix(1700000000, 0).UTC(), Min: 1, Max: 3, Avg: 2},
	}, nil
}

func (f *fakeReader) GetColumnLast(_ context.Context, tp timescale.GetColumnLastTemplateParameters) (*timescale.GetColumnLastResponse, error) {
	if f.noLast {
		return nil, fmt.Errorf("%w: %s", timescale.ErrNoData, tp.Column)
	}
	return &timescale.GetColumnLastResponse{Time: time.Unix(1700000060, 0).UTC(), Last: 2.5}, nil
}

func (f *fakeReader) GetAQILast(_ context.Context, tp timescale.GetAQILastTemplateParameters) (*timescale.AQILastResponse, error) {
	if f.noLast {
		return nil, fmt.Errorf("%w: aqi", timescale.ErrNoData)
	}
	return &timescale.AQILastResponse{
		Time:   time.Unix(1700000060, 0).UTC(),
		Result: aqi.Result{AQI: 42, Level: "Good", PrimaryPollutant: "PM2.5"},
	}, nil
}

func (f *fakeReader) GetAQIWindow(_ context.Context, tp timescale.GetAQIWindowTemplateParameters) ([]timescale.AQIPointResponse, error) {
	return []timescale.AQIPointResponse{
		{Time: time.Unix(1700000000, 0).UTC(), Result: aqi.Result{AQI: 42, Level: "Good", PrimaryPollutant: "PM2.5"}, Min: 40, Max: 45},
	}, nil
}

func TestRunPublishesEveryEndpointAndSnapshot(t *testing.T) {
	bucket := memblob.OpenBucket(nil)
	defer func() { _ = bucket.Close() }()

	p := &Publisher{
		Reader:       &fakeReader{},
		Bucket:       bucket,
		Prefix:       "v1",
		CacheControl: "public, max-age=60",
	}

	ctx := context.Background()
	if err := p.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// every per-endpoint object exists, for the raw metrics and the aqi
	names := []string{handlers.AQIName}
	for _, metric := range handlers.Metrics {
		names = append(names, metric.Name)
	}
	for _, name := range names {
		keys := []string{"v1/" + name + "/last.json"}
		for _, window := range handlers.Windows {
			keys = append(keys, "v1/"+name+"/"+window.Name+".json")
		}
		for _, key := range keys {
			exists, err := bucket.Exists(ctx, key)
			if err != nil || !exists {
				t.Errorf("expected %s to exist (err=%v)", key, err)
			}
		}
	}

	// per-endpoint objects are wrapped in an envelope carrying the query time
	envData, err := bucket.ReadAll(ctx, "v1/pm25/24h.json")
	if err != nil {
		t.Fatalf("read pm25/24h: %v", err)
	}
	var envelope struct {
		GeneratedAt time.Time                     `json:"generated_at"`
		Data        []timescale.GetColumnResponse `json:"data"`
	}
	if err := json.Unmarshal(envData, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.GeneratedAt.IsZero() {
		t.Error("expected envelope generated_at to be set")
	}
	if len(envelope.Data) != 1 || envelope.Data[0].Avg != 2 {
		t.Errorf("unexpected envelope data %+v", envelope.Data)
	}

	// the aqi last object flattens the index fields beside time
	aqiData, err := bucket.ReadAll(ctx, "v1/aqi/last.json")
	if err != nil {
		t.Fatalf("read aqi/last: %v", err)
	}
	var aqiEnvelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(aqiData, &aqiEnvelope); err != nil {
		t.Fatalf("unmarshal aqi envelope: %v", err)
	}
	for _, field := range []string{"time", "aqi", "level", "primary_pollutant"} {
		if _, ok := aqiEnvelope.Data[field]; !ok {
			t.Errorf("expected aqi/last data to carry %q, got %v", field, aqiEnvelope.Data)
		}
	}

	// snapshot decodes and contains every metric with every window
	data, err := bucket.ReadAll(ctx, "v1/snapshot.json")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot.GeneratedAt.IsZero() {
		t.Error("expected generated_at to be set")
	}
	if !snapshot.GeneratedAt.Equal(envelope.GeneratedAt) {
		t.Errorf("snapshot generated_at %v differs from envelope %v", snapshot.GeneratedAt, envelope.GeneratedAt)
	}
	if len(snapshot.Metrics) != len(handlers.Metrics) {
		t.Errorf("expected %d metrics in snapshot, got %d", len(handlers.Metrics), len(snapshot.Metrics))
	}
	for name, ms := range snapshot.Metrics {
		if len(ms.Windows) != len(handlers.Windows) {
			t.Errorf("metric %s: expected %d windows, got %d", name, len(handlers.Windows), len(ms.Windows))
		}
		if ms.Last == nil || ms.Last.Last != 2.5 {
			t.Errorf("metric %s: unexpected last %+v", name, ms.Last)
		}
	}
	if len(snapshot.AQI.Windows) != len(handlers.Windows) {
		t.Errorf("aqi: expected %d windows, got %d", len(handlers.Windows), len(snapshot.AQI.Windows))
	}
	if snapshot.AQI.Last == nil || snapshot.AQI.Last.AQI != 42 || snapshot.AQI.Last.PrimaryPollutant != "PM2.5" {
		t.Errorf("aqi: unexpected last %+v", snapshot.AQI.Last)
	}

	// content type and cache control land on the object
	attrs, err := bucket.Attributes(ctx, "v1/snapshot.json")
	if err != nil {
		t.Fatalf("attributes: %v", err)
	}
	if attrs.ContentType != "application/json" {
		t.Errorf("expected content type application/json, got %q", attrs.ContentType)
	}
	if attrs.CacheControl != "public, max-age=60" {
		t.Errorf("expected cache control 'public, max-age=60', got %q", attrs.CacheControl)
	}
}

func TestRunSkipsLastObjectsWhenThereIsNoData(t *testing.T) {
	bucket := memblob.OpenBucket(nil)
	defer func() { _ = bucket.Close() }()

	p := &Publisher{
		Reader: &fakeReader{noLast: true},
		Bucket: bucket,
		Prefix: "v1",
	}

	ctx := context.Background()
	if err := p.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, key := range []string{"v1/pm25/last.json", "v1/aqi/last.json"} {
		exists, err := bucket.Exists(ctx, key)
		if err != nil {
			t.Fatalf("exists %s: %v", key, err)
		}
		if exists {
			t.Errorf("expected %s to be skipped when there is no data", key)
		}
	}

	data, err := bucket.ReadAll(ctx, "v1/snapshot.json")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot.Metrics["pm25"].Last != nil || snapshot.AQI.Last != nil {
		t.Error("expected snapshot last fields to be omitted when there is no data")
	}
	if len(snapshot.AQI.Windows) != len(handlers.Windows) {
		t.Errorf("aqi: expected %d windows, got %d", len(handlers.Windows), len(snapshot.AQI.Windows))
	}
}
