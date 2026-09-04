package handlers

import (
	"log/slog"
	"net/http"

	"alpineworks.io/rfc9457"
)

// writeProblem writes an RFC 9457 problem+json error response.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	problem := rfc9457.NewRFC9457(
		rfc9457.WithTitle(title),
		rfc9457.WithDetail(detail),
		rfc9457.WithInstance(r.URL.Path),
		rfc9457.WithStatus(status),
	)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	problemJSON, err := problem.ToJSON()
	if err != nil {
		slog.Error("failed to marshal problem", slog.String("error", err.Error()))
		return
	}

	if _, err := w.Write([]byte(problemJSON)); err != nil {
		slog.Error("failed to write problem", slog.String("error", err.Error()))
	}
}
