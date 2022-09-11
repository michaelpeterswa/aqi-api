package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/michaelpeterswa/aqi-api/internal/handlers"
	"github.com/michaelpeterswa/aqi-api/internal/influx"
	"github.com/michaelpeterswa/aqi-api/internal/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func main() {
	logger, err := logging.InitZap()
	if err != nil {
		log.Panicf("could not acquire zap logger: %s", err.Error())
	}
	logger.Info("aqi-api init...")

	influxToken := os.Getenv("INFLUX_TOKEN")
	if influxToken == "" {
		logger.Fatal("INFLUX_TOKEN not set")
	}

	client := influx.InitInflux(influxToken)
	aqiHandler := handlers.NewAQIHandler(logger, client)

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
