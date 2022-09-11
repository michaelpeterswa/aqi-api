package influx

import (
	"context"
	"fmt"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
)

type InfluxConn struct {
	Conn        influxdb2.Client
	QueryClient api.QueryAPI
}

var (
	ErrCouldNotQueryInflux       = fmt.Errorf("could not query influx")
	ErrCouldNotParseInfluxResult = fmt.Errorf("could not parse influx result")
	ErrUnknownQueryError         = fmt.Errorf("unknown query error")
	ErrUnknownError              = fmt.Errorf("unknown error")
)

func InitInflux(token string) *InfluxConn {
	// Create a new client using an InfluxDB server base URL and an authentication token
	client := influxdb2.NewClient("http://10.0.0.10:8086", token)
	queryClient := client.QueryAPI("main")
	return &InfluxConn{
		Conn:        client,
		QueryClient: queryClient,
	}
}

func (ic *InfluxConn) Close() {
	ic.Conn.Close()
}

func (ic *InfluxConn) GetPM25S(ctx context.Context) (float64, error) {
	query := `
	from(bucket: "sensors")
		|> range(start: -24h)
		|> filter(fn: (r) => r["_measurement"] == "sensor-readings")
		|> filter(fn: (r) => r["type"] == "sensors")
		|> filter(fn: (r) => r["location"] == "home")
		|> filter(fn: (r) => r["room"] == "bedroom")
		|> filter(fn: (r) => r["name"] == "pmsa003i")
		|> filter(fn: (r) => r["_field"] == "pm25s")
		|> mean()
	`

	result, err := ic.QueryClient.Query(context.Background(), query)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrCouldNotQueryInflux, err)
	}
	for result.Next() {
		val := result.Record().Value()
		switch v := val.(type) {
		case float64:
			return v, nil
		default:
			return 0, ErrCouldNotParseInfluxResult
		}
	}
	if result.Err() != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnknownQueryError, result.Err())
	}
	return 0, ErrUnknownError
}

func (ic *InfluxConn) GetPM100S(ctx context.Context) (float64, error) {
	query := `
	from(bucket: "sensors")
		|> range(start: -24h)
		|> filter(fn: (r) => r["_measurement"] == "sensor-readings")
		|> filter(fn: (r) => r["type"] == "sensors")
		|> filter(fn: (r) => r["location"] == "home")
		|> filter(fn: (r) => r["room"] == "bedroom")
		|> filter(fn: (r) => r["name"] == "pmsa003i")
		|> filter(fn: (r) => r["_field"] == "pm100s")
		|> mean()
	`

	result, err := ic.QueryClient.Query(context.Background(), query)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrCouldNotQueryInflux, err)
	}
	for result.Next() {
		val := result.Record().Value()
		switch v := val.(type) {
		case float64:
			return v, nil
		default:
			return 0, ErrCouldNotParseInfluxResult
		}
	}
	if result.Err() != nil {
		return 0, fmt.Errorf("%w: %v", ErrUnknownQueryError, result.Err())
	}
	return 0, ErrUnknownError
}
