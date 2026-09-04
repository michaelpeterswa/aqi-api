// Package timescale reads the AirGradient series and the inserter's
// precomputed AQI out of TimescaleDB with templated SQL, and optionally
// caches the results in Dragonfly.
package timescale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	_ "embed"

	"github.com/cespare/xxhash/v2"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/michaelpeterswa/aqi-api/internal/aqi"
	"github.com/michaelpeterswa/aqi-api/internal/dragonfly"
)

// ErrNoData is returned when a query that expects at least one reading finds
// none, so callers can answer 404 instead of 500.
var ErrNoData = errors.New("no data")

// Source names the tables to read. Both carry a serial_number column; when
// SerialNumber is set every query is limited to that monitor.
type Source struct {
	// Table holds the raw AirGradient readings.
	Table string
	// AQITable holds the inserter's rolling 24 hour AQI rows.
	AQITable     string
	SerialNumber string
}

// Source values are interpolated into SQL templates. They come from the
// server's configuration, never from request input, but a stray quote in a
// misconfigured value would still break the query, so they are checked once.
var (
	identifierPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	serialNumberPattern = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)
)

func (s Source) validate() error {
	if !identifierPattern.MatchString(s.Table) {
		return fmt.Errorf("invalid table name %q", s.Table)
	}
	if !identifierPattern.MatchString(s.AQITable) {
		return fmt.Errorf("invalid aqi table name %q", s.AQITable)
	}
	if !serialNumberPattern.MatchString(s.SerialNumber) {
		return fmt.Errorf("invalid serial number %q", s.SerialNumber)
	}
	return nil
}

type TimescaleClient struct {
	Pool *pgxpool.Pool
	Dfly *dragonfly.DragonflyClient

	source       Source
	queryTimeout time.Duration

	getColumnTemplate     *template.Template
	getColumnLastTemplate *template.Template
	getAQILastTemplate    *template.Template
	getAQIWindowTemplate  *template.Template
}

//go:embed queries/getcolumn.pgsql.gotmpl
var getColumnTemplate string

//go:embed queries/getcolumnlast.pgsql.gotmpl
var getColumnLastTemplate string

//go:embed queries/getaqilast.pgsql.gotmpl
var getAQILastTemplate string

//go:embed queries/getaqiwindow.pgsql.gotmpl
var getAQIWindowTemplate string

// Template parameters select a table, column and bucketing. Column and window
// values come from the server-side metric and window tables, never from
// request input, so they can be interpolated into the SQL templates safely.

type GetColumnTemplateParameters struct {
	Table            string
	SerialNumber     string
	Column           string
	TimeBucket       string
	LookbackInterval string
}

func (t *GetColumnTemplateParameters) String() string {
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		strings.ReplaceAll(t.Table, " ", ""),
		t.SerialNumber,
		strings.ReplaceAll(t.Column, " ", ""),
		strings.ReplaceAll(t.TimeBucket, " ", ""),
		strings.ReplaceAll(t.LookbackInterval, " ", ""))
}

func (t *GetColumnTemplateParameters) Hash() string {
	return strconv.FormatUint(xxhash.Sum64String(t.String()), 16)
}

type GetColumnLastTemplateParameters struct {
	Table        string
	SerialNumber string
	Column       string
}

func (t *GetColumnLastTemplateParameters) String() string {
	return fmt.Sprintf("%s-%s-%s-last",
		strings.ReplaceAll(t.Table, " ", ""),
		t.SerialNumber,
		strings.ReplaceAll(t.Column, " ", ""))
}

func (t *GetColumnLastTemplateParameters) Hash() string {
	return strconv.FormatUint(xxhash.Sum64String(t.String()), 16)
}

type GetAQILastTemplateParameters struct {
	AQITable     string
	SerialNumber string
}

func (t *GetAQILastTemplateParameters) String() string {
	return fmt.Sprintf("%s-%s-aqi-last",
		strings.ReplaceAll(t.AQITable, " ", ""),
		t.SerialNumber)
}

func (t *GetAQILastTemplateParameters) Hash() string {
	return strconv.FormatUint(xxhash.Sum64String(t.String()), 16)
}

type GetAQIWindowTemplateParameters struct {
	AQITable         string
	SerialNumber     string
	TimeBucket       string
	LookbackInterval string
}

func (t *GetAQIWindowTemplateParameters) String() string {
	return fmt.Sprintf("%s-%s-aqi-%s-%s",
		strings.ReplaceAll(t.AQITable, " ", ""),
		t.SerialNumber,
		strings.ReplaceAll(t.TimeBucket, " ", ""),
		strings.ReplaceAll(t.LookbackInterval, " ", ""))
}

func (t *GetAQIWindowTemplateParameters) Hash() string {
	return strconv.FormatUint(xxhash.Sum64String(t.String()), 16)
}

type GetColumnResponse struct {
	Time time.Time `json:"time"`
	Min  float64   `json:"min"`
	Max  float64   `json:"max"`
	Avg  float64   `json:"avg"`
}

type GetColumnLastResponse struct {
	Time time.Time `json:"time"`
	Last float64   `json:"last"`
}

// AQILastResponse is the newest AQI row the inserter wrote: a rolling
// 24 hour index as of that time.
type AQILastResponse struct {
	Time time.Time `json:"time"`
	aqi.Result
}

// AQIPointResponse is one time bucket of the AQI series. AQI is the bucket's
// mean index rounded to an integer, Level is that mean's EPA category, and
// PrimaryPollutant is the pollutant reported most often within the bucket.
type AQIPointResponse struct {
	Time time.Time `json:"time"`
	aqi.Result
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type TimescaleClientOption func(*TimescaleClient)

func WithDragonflyClient(dfly *dragonfly.DragonflyClient) TimescaleClientOption {
	return func(c *TimescaleClient) {
		c.Dfly = dfly
	}
}

// NewTimescaleClient connects to TimescaleDB, pings it, and parses the query
// templates.
func NewTimescaleClient(ctx context.Context, connString string, source Source, queryTimeout time.Duration, opts ...TimescaleClientOption) (*TimescaleClient, error) {
	if err := source.validate(); err != nil {
		return nil, err
	}

	getColumnTmpl, err := template.New("getColumn").Parse(getColumnTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse getColumn template: %w", err)
	}

	getColumnLastTmpl, err := template.New("getColumnLast").Parse(getColumnLastTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse getColumnLast template: %w", err)
	}

	getAQILastTmpl, err := template.New("getAQILast").Parse(getAQILastTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse getAQILast template: %w", err)
	}

	getAQIWindowTmpl, err := template.New("getAQIWindow").Parse(getAQIWindowTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse getAQIWindow template: %w", err)
	}

	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse connection string: %w", err)
	}

	// Span every query so it nests under the request span. A no-op while
	// tracing is disabled.
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = pool.Ping(pingCtx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	c := &TimescaleClient{
		Pool:                  pool,
		source:                source,
		queryTimeout:          queryTimeout,
		getColumnTemplate:     getColumnTmpl,
		getColumnLastTemplate: getColumnLastTmpl,
		getAQILastTemplate:    getAQILastTmpl,
		getAQIWindowTemplate:  getAQIWindowTmpl,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func (c *TimescaleClient) Close() {
	c.Pool.Close()
}

// ColumnParameters fills in the source for a bucketed column query.
func (c *TimescaleClient) ColumnParameters(column string, timeBucket string, lookbackInterval string) GetColumnTemplateParameters {
	return GetColumnTemplateParameters{
		Table:            c.source.Table,
		SerialNumber:     c.source.SerialNumber,
		Column:           column,
		TimeBucket:       timeBucket,
		LookbackInterval: lookbackInterval,
	}
}

// ColumnLastParameters fills in the source for a newest-value query.
func (c *TimescaleClient) ColumnLastParameters(column string) GetColumnLastTemplateParameters {
	return GetColumnLastTemplateParameters{
		Table:        c.source.Table,
		SerialNumber: c.source.SerialNumber,
		Column:       column,
	}
}

// AQILastParameters fills in the source for the newest-AQI query.
func (c *TimescaleClient) AQILastParameters() GetAQILastTemplateParameters {
	return GetAQILastTemplateParameters{
		AQITable:     c.source.AQITable,
		SerialNumber: c.source.SerialNumber,
	}
}

// AQIWindowParameters fills in the source for a bucketed AQI query.
func (c *TimescaleClient) AQIWindowParameters(timeBucket string, lookbackInterval string) GetAQIWindowTemplateParameters {
	return GetAQIWindowTemplateParameters{
		AQITable:         c.source.AQITable,
		SerialNumber:     c.source.SerialNumber,
		TimeBucket:       timeBucket,
		LookbackInterval: lookbackInterval,
	}
}

// hasher is the part of every template-parameter type the cache needs.
type hasher interface {
	Hash() string
}

// cacheGet returns the cached value for tp, if the cache is configured and
// holds one. Cache failures are logged, never surfaced: the query still runs.
func cacheGet[T any](ctx context.Context, dfly *dragonfly.DragonflyClient, tp hasher) (T, bool) {
	var zero T
	if dfly == nil {
		return zero, false
	}

	res, err := dfly.GetClient().Get(ctx, fmt.Sprintf("%s-%s", dfly.KeyPrefix, tp.Hash())).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Error("failed to get from dragonfly", slog.String("error", err.Error()))
		}
		return zero, false
	}

	var value T
	err = json.Unmarshal([]byte(res), &value)
	if err != nil {
		slog.Error("failed to unmarshal from dragonfly", slog.String("error", err.Error()))
		return zero, false
	}

	return value, true
}

// cacheSet stores v for tp, if the cache is configured.
func cacheSet(ctx context.Context, dfly *dragonfly.DragonflyClient, tp hasher, v any) {
	if dfly == nil {
		return
	}

	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("failed to marshal to dragonfly", slog.String("error", err.Error()))
		return
	}

	err = dfly.GetClient().Set(ctx, fmt.Sprintf("%s-%s", dfly.KeyPrefix, tp.Hash()), data, dfly.CacheResultsDuration).Err()
	if err != nil {
		slog.Error("failed to set to dragonfly", slog.String("error", err.Error()))
	}
}

// render executes a query template and logs the SQL at debug level.
func render(tmpl *template.Template, tp any) (string, error) {
	query := bytes.NewBuffer(nil)
	err := tmpl.Execute(query, tp)
	if err != nil {
		return "", fmt.Errorf("execute %s template: %w", tmpl.Name(), err)
	}

	slog.Debug("query", slog.String("query", query.String()))
	return query.String(), nil
}

// GetColumn returns min/max/avg buckets for a column over the lookback
// interval. The slice is never nil so an empty window serializes as [].
func (c *TimescaleClient) GetColumn(ctx context.Context, tp GetColumnTemplateParameters) ([]GetColumnResponse, error) {
	if cached, ok := cacheGet[[]GetColumnResponse](ctx, c.Dfly, &tp); ok {
		return cached, nil
	}

	query, err := render(c.getColumnTemplate, tp)
	if err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	rows, err := c.Pool.Query(queryCtx, query)
	if err != nil {
		return nil, fmt.Errorf("get %s for the last %s: %w", tp.Column, tp.LookbackInterval, err)
	}
	defer rows.Close()

	responses := []GetColumnResponse{}
	for rows.Next() {
		var row GetColumnResponse
		err := rows.Scan(&row.Time, &row.Avg, &row.Min, &row.Max)
		if err != nil {
			return nil, fmt.Errorf("scan %s row: %w", tp.Column, err)
		}
		responses = append(responses, row)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("read %s rows: %w", tp.Column, rows.Err())
	}

	cacheSet(ctx, c.Dfly, &tp, responses)

	return responses, nil
}

// GetColumnLast returns the newest non-null value of a column.
func (c *TimescaleClient) GetColumnLast(ctx context.Context, tp GetColumnLastTemplateParameters) (*GetColumnLastResponse, error) {
	if cached, ok := cacheGet[GetColumnLastResponse](ctx, c.Dfly, &tp); ok {
		return &cached, nil
	}

	query, err := render(c.getColumnLastTemplate, tp)
	if err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	var response GetColumnLastResponse
	err = c.Pool.QueryRow(queryCtx, query).Scan(&response.Time, &response.Last)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrNoData, tp.Column)
		}
		return nil, fmt.Errorf("get last %s: %w", tp.Column, err)
	}

	cacheSet(ctx, c.Dfly, &tp, response)

	return &response, nil
}

// GetAQILast returns the newest AQI row.
func (c *TimescaleClient) GetAQILast(ctx context.Context, tp GetAQILastTemplateParameters) (*AQILastResponse, error) {
	if cached, ok := cacheGet[AQILastResponse](ctx, c.Dfly, &tp); ok {
		return &cached, nil
	}

	query, err := render(c.getAQILastTemplate, tp)
	if err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	// level and primary_pollutant are nullable in the schema.
	var (
		response         AQILastResponse
		index            float64
		level            *string
		primaryPollutant *string
	)
	err = c.Pool.QueryRow(queryCtx, query).Scan(&response.Time, &index, &level, &primaryPollutant)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: aqi", ErrNoData)
		}
		return nil, fmt.Errorf("get last aqi: %w", err)
	}

	response.AQI = int64(math.Round(index))
	if level != nil {
		response.Level = *level
	} else if response.Level, err = aqi.Level(response.AQI); err != nil {
		return nil, err
	}
	if primaryPollutant != nil {
		response.PrimaryPollutant = *primaryPollutant
	}

	cacheSet(ctx, c.Dfly, &tp, response)

	return &response, nil
}

// GetAQIWindow returns the AQI series bucketed over the lookback interval.
// The slice is never nil so an empty window serializes as [].
func (c *TimescaleClient) GetAQIWindow(ctx context.Context, tp GetAQIWindowTemplateParameters) ([]AQIPointResponse, error) {
	if cached, ok := cacheGet[[]AQIPointResponse](ctx, c.Dfly, &tp); ok {
		return cached, nil
	}

	query, err := render(c.getAQIWindowTemplate, tp)
	if err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()

	rows, err := c.Pool.Query(queryCtx, query)
	if err != nil {
		return nil, fmt.Errorf("get aqi for the last %s: %w", tp.LookbackInterval, err)
	}
	defer rows.Close()

	responses := []AQIPointResponse{}
	for rows.Next() {
		var (
			point            AQIPointResponse
			avg              float64
			primaryPollutant *string
		)
		err := rows.Scan(&point.Time, &avg, &point.Min, &point.Max, &primaryPollutant)
		if err != nil {
			return nil, fmt.Errorf("scan aqi row: %w", err)
		}

		point.AQI = int64(math.Round(avg))
		point.Level, err = aqi.Level(point.AQI)
		if err != nil {
			// One off-scale bucket (a sensor glitch) should not blank the
			// whole window.
			slog.Warn("skipping bucket with no aqi level", slog.Time("time", point.Time), slog.String("error", err.Error()))
			continue
		}
		if primaryPollutant != nil {
			point.PrimaryPollutant = *primaryPollutant
		}

		responses = append(responses, point)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("read aqi rows: %w", rows.Err())
	}

	cacheSet(ctx, c.Dfly, &tp, responses)

	return responses, nil
}
