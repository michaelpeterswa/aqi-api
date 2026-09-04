# AQI API

An HTTP API and snapshot publisher that serves particulate readings and the US EPA Air Quality Index out of TimescaleDB, so other applications can show air quality without writing SQL. Reads the `sensors.pmsa003i` hypertable (a PMSA003I sensor's `pm25s` and `pm100s` columns) and computes the index with [goaqi](https://github.com/michaelpeterswa/goaqi). Structure modeled on [tempest-influxdb-api](https://github.com/michaelpeterswa/tempest-influxdb-api).

## Commands

- `aqi-api serve` — run the HTTP API (default container command)
- `aqi-api publish` — compute every metric x window + last, plus the AQI, and upload the JSON to a public bucket ([gocloud.dev](https://gocloud.dev) URL, e.g. `gs://...` or `file:///...`), for static consumers like a GitHub Pages site. Publishes per-endpoint objects (`v1/pm25/24h.json`, `v1/pm25/last.json`, `v1/aqi/last.json`, ...) plus a combined `v1/snapshot.json`, with `Cache-Control` set on every object. Every object carries the time the run's queries started — per-endpoint files as an envelope `{"generated_at": ..., "data": <same shape as the HTTP API>}`, the snapshot as a top-level `generated_at` — so consumers can show recency without a second request. Meant to run on a schedule. `gs://` buckets authenticate with Application Default Credentials.

## Endpoints

All endpoints live under `/api/v1` and return JSON. Errors are RFC 9457 `application/problem+json`. A missing reading is a `404`.

For each raw metric below:

- `GET /api/v1/{metric}/last` — newest value: `{"time": ..., "last": ...}`
- `GET /api/v1/{metric}/{window}` — bucketed history: `[{"time": ..., "min": ..., "max": ..., "avg": ...}, ...]`

| Metric  | TimescaleDB Column | Unit  |
|---------|--------------------|-------|
| `pm25`  | `pm25s`            | µg/m³ |
| `pm100` | `pm100s`           | µg/m³ |

The computed index has the same route shape:

- `GET /api/v1/aqi/last` — AQI from the 24 hour average concentrations, which is how the EPA defines the index: `{"time": ..., "aqi": ..., "level": ..., "primary_pollutant": ..., "pm25_avg": ..., "pm100_avg": ...}`. `time` is the newest reading in the window.
- `GET /api/v1/aqi/{window}` — AQI of each bucket, from that bucket's average concentrations: `[{"time": ..., "aqi": ..., "level": ..., "primary_pollutant": ...}, ...]`

Windows and their bucket sizes: `12h` (30m), `24h` (1h), `7d` (6h), `30d` (1d), `90d` (1d).

`GET /healthcheck` answers `{"healthy": true, "hostname": ...}` (served by [ootel](https://alpineworks.io/ootel), outside the API key check). Prometheus metrics are served on `METRICS_PORT`.

## Configuration

All configuration is via environment variables.

| Environment Variable      | Description                                        | Required | Default              |
|---------------------------|----------------------------------------------------|----------|----------------------|
| `TIMESCALE_DSN`           | PostgreSQL / TimescaleDB connection string         | Yes      | -                    |
| `TIMESCALE_TABLE`         | Schema-qualified hypertable to read                | No       | `sensors.pmsa003i`   |
| `QUERY_TIMEOUT`           | Timeout for one database query                     | No       | `10s`                |
| `DRAGONFLY_HOST`          | Dragonfly (or Redis) host; enables response caching for `serve` when set | No | -      |
| `DRAGONFLY_PORT`          | Dragonfly port                                     | No       | `6379`               |
| `DRAGONFLY_AUTH`          | Dragonfly password                                 | No       | -                    |
| `DRAGONFLY_KEY_PREFIX`    | Cache key prefix                                   | No       | `aqi`                |
| `CACHE_RESULTS_DURATION`  | Cache TTL for query results                        | No       | `5m`                 |
| `PORT`                    | HTTP listen port                                   | No       | `8080`               |
| `PUBLISH_BUCKET_URL`      | gocloud.dev blob URL the publish command writes to | publish  | -                    |
| `PUBLISH_PREFIX`          | Key prefix inside the bucket                       | No       | `v1`                 |
| `PUBLISH_CACHE_CONTROL`   | Cache-Control header set on uploaded objects       | No       | `public, max-age=60` |
| `AUTHENTICATION_ENABLED`  | Require an API key on `/api` routes                | No       | `false`              |
| `API_KEYS`                | Comma-separated list of accepted `X-API-Key` values| No       | -                    |
| `LOG_LEVEL`               | Log level (`debug`, `info`, `warn`, `error`)       | No       | `error`              |
| `METRICS_ENABLED`         | Serve OpenTelemetry metrics                        | No       | `true`               |
| `METRICS_PORT`            | Prometheus metrics port                            | No       | `8081`               |
| `LOCAL`                   | Export metrics via OTLP gRPC instead of Prometheus | No       | `false`              |
| `TRACING_ENABLED`         | Enable OpenTelemetry tracing                       | No       | `false`              |
| `TRACING_SAMPLERATE`      | Trace sample rate                                  | No       | `0.01`               |
| `TRACING_SERVICE`         | Trace service name                                 | No       | `aqi-api`            |
| `TRACING_VERSION`         | Trace service version                              | No       | -                    |

## Examples

### Docker Compose

```yaml
services:
  aqi-api:
    image: "ghcr.io/michaelpeterswa/aqi-api:latest"
    environment:
      TIMESCALE_DSN: "postgres://user:password@timescale:5432/sensors"
      DRAGONFLY_HOST: "dragonfly"
      LOG_LEVEL: "info"
    ports:
      - 8080:8080
      - 8081:8081

  dragonfly:
    image: "docker.dragonflydb.io/dragonflydb/dragonfly:latest"
    ulimits:
      memlock: -1
```

### Query

```console
$ curl -s localhost:8080/api/v1/aqi/last
{"time":"2026-09-04T19:37:00Z","aqi":42,"level":"Good","primary_pollutant":"PM2.5","pm25_avg":10.1,"pm100_avg":12.4}

$ curl -s localhost:8080/api/v1/pm25/24h
[{"time":"2026-09-04T00:00:00Z","min":8,"max":14,"avg":10.5}, ...]
```

### Publish to a local directory

```console
$ PUBLISH_BUCKET_URL=file:///tmp/aqi-out aqi-api publish
$ cat /tmp/aqi-out/v1/aqi/last.json
```

## License

MIT License - see [LICENSE](LICENSE) file.
