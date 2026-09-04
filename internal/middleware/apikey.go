package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	"alpineworks.io/rfc9457"
)

type AuthenticationMode int

type AuthenticationMiddlewareClient struct {
	Mode    AuthenticationMode
	APIKeys []string
}

type AuthenticationMiddlewareOption func(*AuthenticationMiddlewareClient)

func WithAPIKeys(apiKeys []string) AuthenticationMiddlewareOption {
	return func(c *AuthenticationMiddlewareClient) {
		c.Mode = AuthenticationModeAPIKey
		c.APIKeys = apiKeys
	}
}

func NewAuthenticationMiddlewareClient(opts ...AuthenticationMiddlewareOption) *AuthenticationMiddlewareClient {
	c := &AuthenticationMiddlewareClient{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

const (
	AuthenticationModeAPIKey AuthenticationMode = iota
)

func (amc *AuthenticationMiddlewareClient) AuthenticationMiddleware(next http.Handler) http.Handler {
	switch amc.Mode {
	case AuthenticationModeAPIKey:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")

			valid := false
			for _, key := range amc.APIKeys {
				if apiKey == key {
					valid = true
					break
				}
			}

			if !valid {
				problem := rfc9457.NewRFC9457(
					rfc9457.WithTitle("invalid api key"),
					rfc9457.WithDetail(fmt.Sprintf("%s is not a valid api key", apiKey)),
					rfc9457.WithInstance(r.URL.Path),
					rfc9457.WithStatus(http.StatusUnauthorized),
				)
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusUnauthorized)

				problemJSON, err := problem.ToJSON()
				if err != nil {
					slog.Error("failed to marshal problem", slog.String("error", err.Error()))
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				_, err = w.Write([]byte(problemJSON))
				if err != nil {
					slog.Error("failed to write problem", slog.String("error", err.Error()))
					w.WriteHeader(http.StatusInternalServerError)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	default:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			problem := rfc9457.NewRFC9457(
				rfc9457.WithTitle("invalid authentication mode"),
				rfc9457.WithDetail("authentication middleware is misconfigured"),
				rfc9457.WithInstance(r.URL.Path),
				rfc9457.WithStatus(http.StatusInternalServerError),
			)
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)

			problemJSON, err := problem.ToJSON()
			if err != nil {
				slog.Error("failed to marshal problem", slog.String("error", err.Error()))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			_, err = w.Write([]byte(problemJSON))
			if err != nil {
				slog.Error("failed to write problem", slog.String("error", err.Error()))
				w.WriteHeader(http.StatusInternalServerError)
			}
		})

	}
}
