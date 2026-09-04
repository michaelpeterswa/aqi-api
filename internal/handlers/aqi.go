package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/michaelpeterswa/aqi-api/internal/timescale"
)

// Metric maps an API metric name to its column in the AirGradient table.
type Metric struct {
	Name   string
	Column string
}

// Metrics is the set of AirGradient readings the API serves, in route order.
// Diagnostic columns (boot counts, firmware, model) are left out.
var Metrics = []Metric{
	{Name: "pm1", Column: "pm01"},
	{Name: "pm25", Column: "pm02"},
	{Name: "pm100", Column: "pm10"},
	{Name: "pm003_count", Column: "pm003_count"},
	{Name: "co2", Column: "rco2"},
	{Name: "temperature", Column: "atmp"},
	{Name: "temperature_compensated", Column: "atmp_compensated"},
	{Name: "humidity", Column: "rhum"},
	{Name: "humidity_compensated", Column: "rhum_compensated"},
	{Name: "tvoc_index", Column: "tvoc_index"},
	{Name: "tvoc_raw", Column: "tvoc_raw"},
	{Name: "nox_index", Column: "nox_index"},
	{Name: "nox_raw", Column: "nox_raw"},
	{Name: "wifi", Column: "wifi"},
}

// Window is one lookback tier with its bucket size, both as Postgres
// interval strings.
type Window struct {
	Name             string
	LookbackInterval string
	TimeBucket       string
}

// Windows mirrors the tempest-influxdb-api tiers: the bucket grows with the
// lookback so the point count stays bounded.
var Windows = []Window{
	{Name: "12h", LookbackInterval: "12 hours", TimeBucket: "30 minutes"},
	{Name: "24h", LookbackInterval: "24 hours", TimeBucket: "1 hour"},
	{Name: "7d", LookbackInterval: "7 days", TimeBucket: "6 hours"},
	{Name: "30d", LookbackInterval: "30 days", TimeBucket: "1 day"},
	{Name: "90d", LookbackInterval: "90 days", TimeBucket: "1 day"},
}

// AQIName is the route segment for the index. It sits beside the raw metrics
// but comes from the inserter's AQI table, where every row is a rolling
// 24 hour index.
const AQIName = "aqi"

type AQIHandler struct {
	timescaleClient *timescale.TimescaleClient
}

func NewAQIHandler(timescaleClient *timescale.TimescaleClient) *AQIHandler {
	return &AQIHandler{timescaleClient: timescaleClient}
}

// GetColumnWindow returns the handler for one metric and window tier.
func (h *AQIHandler) GetColumnWindow(metric Metric, window Window) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values, err := h.timescaleClient.GetColumn(r.Context(), h.timescaleClient.ColumnParameters(metric.Column, window.TimeBucket, window.LookbackInterval))
		if err != nil {
			writeProblem(w, r, http.StatusInternalServerError,
				fmt.Sprintf("failed to get %s data", window.Name),
				fmt.Sprintf("error getting data for %s: %s", metric.Name, err.Error()))
			return
		}

		writeJSON(w, r, metric.Name, values)
	}
}

// GetColumnLast returns the handler for one metric's newest value.
func (h *AQIHandler) GetColumnLast(metric Metric) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := h.timescaleClient.GetColumnLast(r.Context(), h.timescaleClient.ColumnLastParameters(metric.Column))
		if err != nil {
			writeProblem(w, r, statusFor(err),
				"failed to get last data",
				fmt.Sprintf("error getting data for %s: %s", metric.Name, err.Error()))
			return
		}

		writeJSON(w, r, metric.Name, value)
	}
}

// GetAQILast returns the handler for the newest AQI row.
func (h *AQIHandler) GetAQILast() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value, err := h.timescaleClient.GetAQILast(r.Context(), h.timescaleClient.AQILastParameters())
		if err != nil {
			writeProblem(w, r, statusFor(err),
				"failed to get last aqi",
				fmt.Sprintf("error getting aqi: %s", err.Error()))
			return
		}

		writeJSON(w, r, AQIName, value)
	}
}

// GetAQIWindow returns the handler for the AQI series of one window tier.
func (h *AQIHandler) GetAQIWindow(window Window) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values, err := h.timescaleClient.GetAQIWindow(r.Context(), h.timescaleClient.AQIWindowParameters(window.TimeBucket, window.LookbackInterval))
		if err != nil {
			writeProblem(w, r, http.StatusInternalServerError,
				fmt.Sprintf("failed to get %s aqi", window.Name),
				fmt.Sprintf("error getting aqi: %s", err.Error()))
			return
		}

		writeJSON(w, r, AQIName, values)
	}
}

// statusFor maps a missing reading to 404; everything else is a server error.
func statusFor(err error) int {
	if errors.Is(err, timescale.ErrNoData) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// writeJSON marshals and writes a success response.
func writeJSON(w http.ResponseWriter, r *http.Request, name string, v any) {
	res, err := json.Marshal(v)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError,
			"failed to marshal data",
			fmt.Sprintf("error marshalling data for %s: %s", name, err.Error()))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(res); err != nil {
		writeProblem(w, r, http.StatusInternalServerError,
			"failed to write data",
			fmt.Sprintf("error writing data for %s: %s", name, err.Error()))
	}
}
