package timescale

import (
	"context"
	"fmt"

	_ "embed"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TimescaleClient struct {
	pool *pgxpool.Pool
}

func InitTimescale(ctx context.Context, dsn string) (*TimescaleClient, func(), error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create pool: %w", err)
	}

	tc := &TimescaleClient{
		pool: pool,
	}

	return tc, tc.Close, nil
}

func (tc *TimescaleClient) Close() {
	tc.pool.Close()
}

type PM25sRow struct {
	//lint:ignore U1000 because it's needed for pgx.RowToStructByName
	Avg_pm25s float64
}

//go:embed queries/get_pm25s.pgsql
var getPM25SQuery string

func (tc *TimescaleClient) GetPM25S(ctx context.Context) (float64, error) {
	rows, err := tc.pool.Query(ctx, getPM25SQuery)
	if err != nil {
		return 0, fmt.Errorf("could not query timescale: %w", err)
	}
	defer rows.Close()

	aqiRow, err := pgx.CollectExactlyOneRow[PM25sRow](rows, pgx.RowToStructByName[PM25sRow])
	if err != nil {
		return 0, fmt.Errorf("could not collect exactly one row: %w", err)
	}

	return aqiRow.Avg_pm25s, nil
}

type PM100sRow struct {
	//lint:ignore U1000 because it's needed for pgx.RowToStructByName
	Avg_pm100s float64
}

//go:embed queries/get_pm100s.pgsql
var getPM100SQuery string

func (tc *TimescaleClient) GetPM100S(ctx context.Context) (float64, error) {
	rows, err := tc.pool.Query(ctx, getPM100SQuery)
	if err != nil {
		return 0, fmt.Errorf("could not query timescale: %w", err)
	}
	defer rows.Close()

	aqiRow, err := pgx.CollectExactlyOneRow[PM100sRow](rows, pgx.RowToStructByName[PM100sRow])
	if err != nil {
		return 0, fmt.Errorf("could not collect exactly one row: %w", err)
	}

	return aqiRow.Avg_pm100s, nil
}
