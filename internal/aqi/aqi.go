// Package aqi turns PM2.5 and PM10 concentrations into a US EPA Air Quality
// Index, so the HTTP handlers and the publisher compute it the same way.
package aqi

import (
	"fmt"
	"math"

	"676f.dev/goaqi"
)

const (
	PollutantPM25  = "PM2.5"
	PollutantPM100 = "PM10.0"
)

// Result is one AQI reading: the index, its EPA category name, and which
// pollutant produced the higher (reported) index.
type Result struct {
	AQI              int64  `json:"aqi"`
	Level            string `json:"level"`
	PrimaryPollutant string `json:"primary_pollutant"`
}

// Compute returns the AQI for a pair of averaged concentrations in ug/m3.
// The EPA method truncates PM2.5 to one decimal place before the breakpoint
// lookup (goaqi truncates PM10 itself); without it an average like 12.05
// lands in the gap between the 12.0 and 12.1 breakpoints and has no index.
func Compute(pm25Avg float64, pm100Avg float64) (Result, error) {
	pm25AQI, err := goaqi.AQIPM25(math.Trunc(pm25Avg*10) / 10)
	if err != nil {
		return Result{}, fmt.Errorf("aqi for pm2.5 %.2f: %w", pm25Avg, err)
	}

	pm100AQI, err := goaqi.AQIPM100(pm100Avg)
	if err != nil {
		return Result{}, fmt.Errorf("aqi for pm10 %.2f: %w", pm100Avg, err)
	}

	result := Result{AQI: pm100AQI, PrimaryPollutant: PollutantPM100}
	if pm25AQI > pm100AQI {
		result = Result{AQI: pm25AQI, PrimaryPollutant: PollutantPM25}
	}

	result.Level, err = goaqi.AQIDesignationFromIndex(result.AQI)
	if err != nil {
		return Result{}, fmt.Errorf("designation for aqi %d: %w", result.AQI, err)
	}

	return result, nil
}
