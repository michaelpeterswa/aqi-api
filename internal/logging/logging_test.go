package logging

import (
	"log/slog"
	"testing"
)

func TestLogLevelToSlogLevel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    slog.Level
		wantErr bool
	}{
		{name: "debug", input: "debug", want: slog.LevelDebug},
		{name: "info", input: "info", want: slog.LevelInfo},
		{name: "warn", input: "warn", want: slog.LevelWarn},
		{name: "error", input: "error", want: slog.LevelError},
		{name: "mixed case", input: "DeBuG", want: slog.LevelDebug},
		{name: "unknown", input: "trace", want: slog.LevelInfo, wantErr: true},
		{name: "empty", input: "", want: slog.LevelInfo, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LogLevelToSlogLevel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LogLevelToSlogLevel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("LogLevelToSlogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
