// Package aqi holds the shape of an Air Quality Index reading and the EPA
// category lookup, shared by the timescale client and the publisher.
package aqi

import (
	"fmt"

	"676f.dev/goaqi"
)

// Result is one AQI reading: the index, its EPA category name, and which
// pollutant produced the higher (reported) index.
type Result struct {
	AQI              int64  `json:"aqi"`
	Level            string `json:"level"`
	PrimaryPollutant string `json:"primary_pollutant"`
}

// Level returns the EPA category name for an index, for points whose index
// is an aggregate (a bucket average) rather than a stored row.
func Level(index int64) (string, error) {
	level, err := goaqi.AQIDesignationFromIndex(index)
	if err != nil {
		return "", fmt.Errorf("designation for aqi %d: %w", index, err)
	}
	return level, nil
}
