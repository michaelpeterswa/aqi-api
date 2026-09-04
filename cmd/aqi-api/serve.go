package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"alpineworks.io/ootel"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"

	"github.com/michaelpeterswa/aqi-api/internal/config"
	"github.com/michaelpeterswa/aqi-api/internal/dragonfly"
	"github.com/michaelpeterswa/aqi-api/internal/handlers"
	"github.com/michaelpeterswa/aqi-api/internal/middleware"
	"github.com/michaelpeterswa/aqi-api/internal/timescale"
)

// runServe runs the HTTP API.
func runServe(ctx context.Context, c *config.Config) error {
	slog.Info("welcome to aqi-api!")

	exporterType := ootel.ExporterTypePrometheus
	if c.Local {
		exporterType = ootel.ExporterTypeOTLPGRPC
	}

	ootelClient := ootel.NewOotelClient(
		ootel.WithMetricConfig(
			ootel.NewMetricConfig(
				c.MetricsEnabled,
				exporterType,
				c.MetricsPort,
			),
		),
		ootel.WithTraceConfig(
			ootel.NewTraceConfig(
				c.TracingEnabled,
				c.TracingSampleRate,
				c.TracingService,
				c.TracingVersion,
			),
		),
	)

	shutdown, err := ootelClient.Init(ctx)
	if err != nil {
		return fmt.Errorf("create ootel client: %w", err)
	}
	defer func() { _ = shutdown(ctx) }()

	// The response cache is optional: without a host the API queries the
	// database on every request, which is fine for a single sensor.
	var timescaleOpts []timescale.TimescaleClientOption
	if c.DragonflyHost != "" {
		dragonflyClient, err := dragonfly.NewDragonflyClient(c.DragonflyHost, c.DragonflyPort, c.DragonflyAuth, c.CacheResultsDuration, c.DragonflyKeyPrefix)
		if err != nil {
			return fmt.Errorf("create dragonfly client: %w", err)
		}
		timescaleOpts = append(timescaleOpts, timescale.WithDragonflyClient(dragonflyClient))
	} else {
		slog.Info("DRAGONFLY_HOST not set, serving uncached")
	}

	timescaleClient, err := timescale.NewTimescaleClient(ctx, c.TimescaleDSN, timescale.Source{Table: c.TimescaleTable, AQITable: c.TimescaleAQITable, SerialNumber: c.SerialNumber}, c.QueryTimeout, timescaleOpts...)
	if err != nil {
		return fmt.Errorf("create timescale client: %w", err)
	}
	defer timescaleClient.Close()

	aqiHandler := handlers.NewAQIHandler(timescaleClient)

	r := mux.NewRouter()
	// Extract the incoming trace context and span each request, so callers'
	// traces continue into this service. A no-op when tracing is disabled.
	r.Use(otelmux.Middleware(c.TracingService))
	apiRouter := r.PathPrefix("/api").Subrouter()
	v1Subrouter := apiRouter.PathPrefix("/v1").Subrouter()

	// last data and windowed min/max/avg buckets for every raw metric
	for _, metric := range handlers.Metrics {
		v1Subrouter.HandleFunc(fmt.Sprintf("/%s/last", metric.Name), aqiHandler.GetColumnLast(metric)).Methods(http.MethodGet)
		for _, window := range handlers.Windows {
			v1Subrouter.HandleFunc(fmt.Sprintf("/%s/%s", metric.Name, window.Name), aqiHandler.GetColumnWindow(metric, window)).Methods(http.MethodGet)
		}
	}

	// the computed index gets the same shape of routes
	v1Subrouter.HandleFunc(fmt.Sprintf("/%s/last", handlers.AQIName), aqiHandler.GetAQILast()).Methods(http.MethodGet)
	for _, window := range handlers.Windows {
		v1Subrouter.HandleFunc(fmt.Sprintf("/%s/%s", handlers.AQIName, window.Name), aqiHandler.GetAQIWindow(window)).Methods(http.MethodGet)
	}

	if c.AuthenticationEnabled {
		authenticationMiddleware := middleware.NewAuthenticationMiddlewareClient(
			middleware.WithAPIKeys(c.APIKeys),
		)
		apiRouter.Use(authenticationMiddleware.AuthenticationMiddleware)
	}

	// ootel registers /healthcheck on the default mux during Init; the more
	// specific pattern wins over the router mounted at "/".
	http.Handle("/", r)

	err = http.ListenAndServe(fmt.Sprintf(":%d", c.Port), nil)
	if err != nil {
		return fmt.Errorf("start http server: %w", err)
	}
	return nil
}
