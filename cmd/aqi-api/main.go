package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/michaelpeterswa/aqi-api/internal/handlers"
	"github.com/michaelpeterswa/aqi-api/internal/logging"
	"github.com/michaelpeterswa/aqi-api/internal/timescale"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	logger, err := logging.InitZap()
	if err != nil {
		log.Panicf("could not acquire zap logger: %s", err.Error())
	}
	logger.Info("aqi-api init...")

	timescaleDSN := os.Getenv("TIMESCALE_DSN")
	if timescaleDSN == "" {
		logger.Fatal("TIMESCALE_ENDPOINT is required")
	}

	timescaleClient, timescaleCloser, err := timescale.InitTimescale(ctx, timescaleDSN)
	if err != nil {
		logger.Fatal("could not initialize timescale client", zap.Error(err))
	}
	defer timescaleCloser()
	aqiHandler := handlers.NewAQIHandler(logger, timescaleClient)

	r := mux.NewRouter()
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.HandleFunc("/aqi", aqiHandler.GetAQI).Methods(http.MethodGet)
	apiRouter.HandleFunc("/pm25s", aqiHandler.GetPM25s).Methods(http.MethodGet)
	apiRouter.HandleFunc("/pm100s", aqiHandler.GetPM100s).Methods(http.MethodGet)
	r.HandleFunc("/healthcheck", handlers.HealthcheckHandler)
	r.Handle("/metrics", promhttp.Handler())
	http.Handle("/", r)
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		logger.Fatal("could not start http server", zap.Error(err))
	}
}
