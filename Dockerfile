# -=-=-=-=-=-=- Compile Image -=-=-=-=-=-=-

FROM golang:1 AS stage-compile

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build ./cmd/aqi-api

# -=-=-=-=- Final Distroless Image -=-=-=-=-

# hadolint ignore=DL3007
FROM gcr.io/distroless/static-debian12:latest AS stage-final

COPY --from=stage-compile /go/src/app/aqi-api /

EXPOSE 8080

CMD ["/aqi-api", "serve"]
