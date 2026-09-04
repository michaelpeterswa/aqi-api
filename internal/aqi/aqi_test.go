package aqi

import "testing"

func TestCompute(t *testing.T) {
	tests := []struct {
		name    string
		pm25    float64
		pm100   float64
		want    Result
		wantErr bool
	}{
		{name: "clean air", pm25: 0, pm100: 0, want: Result{AQI: 0, Level: "Good", PrimaryPollutant: PollutantPM100}},
		{name: "pm2.5 top of good", pm25: 12.0, pm100: 10, want: Result{AQI: 50, Level: "Good", PrimaryPollutant: PollutantPM25}},
		{name: "pm2.5 in breakpoint gap is truncated", pm25: 12.05, pm100: 10, want: Result{AQI: 50, Level: "Good", PrimaryPollutant: PollutantPM25}},
		{name: "pm2.5 top of moderate", pm25: 35.4, pm100: 10, want: Result{AQI: 100, Level: "Moderate", PrimaryPollutant: PollutantPM25}},
		{name: "pm10 dominates", pm25: 5, pm100: 154.9, want: Result{AQI: 100, Level: "Moderate", PrimaryPollutant: PollutantPM100}},
		{name: "unhealthy for sensitive groups", pm25: 55.4, pm100: 0, want: Result{AQI: 150, Level: "Unhealthy for Sensitive Groups", PrimaryPollutant: PollutantPM25}},
		{name: "negative concentration", pm25: -1, pm100: 0, wantErr: true},
		{name: "beyond the scale", pm25: 600, pm100: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compute(tt.pm25, tt.pm100)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Compute(%v, %v) error = %v, wantErr %v", tt.pm25, tt.pm100, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("Compute(%v, %v) = %+v, want %+v", tt.pm25, tt.pm100, got, tt.want)
			}
		})
	}
}
