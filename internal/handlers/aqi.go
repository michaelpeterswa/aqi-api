package handlers

import (
	"encoding/json"
	"net/http"

	"676f.dev/goaqi"
	"github.com/michaelpeterswa/aqi-api/internal/timescale"
	"go.uber.org/zap"
)

type AQIHandler struct {
	TimescaleClient *timescale.TimescaleClient
	Logger          *zap.Logger
}

type AQIResponse struct {
	PrimaryPollutant string `json:"primary_pollutant"`
	Level            string `json:"level"`
	AQI              int64  `json:"aqi"`
}

type PollutantResponse struct {
	Pollutant string  `json:"pollutant"`
	UgPerM3   float64 `json:"ug_per_m3"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewAQIHandler(logger *zap.Logger, tc *timescale.TimescaleClient) *AQIHandler {
	return &AQIHandler{
		TimescaleClient: tc,
	}
}

func (h *AQIHandler) GetAQI(w http.ResponseWriter, r *http.Request) {
	pm25s, err := h.TimescaleClient.GetPM25S(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	pm100s, err := h.TimescaleClient.GetPM100S(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	pm25AQI, err := goaqi.AQIPM25(pm25s)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	pm100AQI, err := goaqi.AQIPM100(pm100s)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	var primaryPollutant string
	var aqi int64
	if pm25AQI > pm100AQI {
		primaryPollutant = "PM2.5"
		aqi = pm25AQI
	} else {
		primaryPollutant = "PM10.0"
		aqi = pm100AQI
	}

	designation, err := goaqi.AQIDesignationFromIndex(aqi)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)

	err = json.NewEncoder(w).Encode(AQIResponse{PrimaryPollutant: primaryPollutant, AQI: aqi, Level: designation})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (h *AQIHandler) GetPM25s(w http.ResponseWriter, r *http.Request) {
	pm25s, err := h.TimescaleClient.GetPM25S(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(PollutantResponse{Pollutant: "PM2.5", UgPerM3: pm25s})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (h *AQIHandler) GetPM100s(w http.ResponseWriter, r *http.Request) {
	pm100s, err := h.TimescaleClient.GetPM100S(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(PollutantResponse{Pollutant: "PM10.0", UgPerM3: pm100s})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
