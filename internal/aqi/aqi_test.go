package aqi

import "testing"

func TestLevel(t *testing.T) {
	tests := []struct {
		name    string
		index   int64
		want    string
		wantErr bool
	}{
		{name: "zero", index: 0, want: "Good"},
		{name: "top of good", index: 50, want: "Good"},
		{name: "bottom of moderate", index: 51, want: "Moderate"},
		{name: "sensitive groups", index: 150, want: "Unhealthy for Sensitive Groups"},
		{name: "unhealthy", index: 151, want: "Unhealthy"},
		{name: "very unhealthy", index: 300, want: "Very Unhealthy"},
		{name: "hazardous", index: 500, want: "Hazardous"},
		{name: "negative", index: -1, wantErr: true},
		{name: "beyond the scale", index: 501, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Level(tt.index)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Level(%d) error = %v, wantErr %v", tt.index, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Level(%d) = %q, want %q", tt.index, got, tt.want)
			}
		})
	}
}
