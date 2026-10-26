# Wrap

Wrap is a fast, open source URL shortener written in pure Go with zero external dependencies. It provides short Base62 aliases alongside stateless HMAC signed redirect tokens so links work reliably across both standalone servers and serverless deployments.

Live instance: `https://wrap.thgenius.xyz`

## Features

- **Zero external dependencies**: Built strictly on the Go standard library (`net/http`, `crypto/hmac`, `crypto/sha256`, `compress/flate`, `crypto/rand`).
- **Dual resolution engine**:
  - **Short aliases (`/:code`)**: 6 character cryptographic Base62 codes or custom aliases backed by a thread safe in-memory store and optional JSON file snapshots.
  - **Stateless tokens (`/w/:token`)**: Deflate compressed URLs signed with 80-bit HMAC-SHA256 authentication tags (`crypto/hmac.Equal`) and embedded expiration timestamps. Stateless links resolve permanently without any database lookup.
- **Hardened SSRF and redirect security**:
  - Rejects non-HTTP(S) schemes (`javascript:`, `data:`, `file:`, etc.), embedded URL credentials, and ASCII control characters.
  - Blocks loopback, RFC 1918 private networks, link-local ranges, CGNAT ranges, cloud metadata endpoints (`169.254.169.254`), and `.local` / `.internal` hostnames.
  - Prevents self-redirect loops back to the active host.
- **Per-IP token bucket rate limiting**:
  - Separate memory-bounded token buckets for link creation (`POST /api/shorten`), visitor redirects (`GET /:code` and `GET /w/:token`), and analytics queries (`GET /api/stats`).
  - Automatically sets `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `Retry-After` headers.
- **Click telemetry and QR generation**:
  - Tracks total click count, creation date, expiration status, last click timestamp, and top referrer hosts.
  - Client-side deterministic visual QR matrix with one-click PNG download.

## Project Structure

```text
wrap-link/
├── api/
│   └── index.go          # Vercel serverless Go entrypoint
├── cmd/
│   └── wrap/
│       └── main.go       # Standalone HTTP server entrypoint
├── pkg/
│   └── wrap/
│       ├── codec.go      # Base62 generator and HMAC + Flate stateless codec
│       ├── ratelimit.go  # Per-IP token bucket rate limiter
│       ├── security.go   # URL validation, SSRF blocks, and HTTP headers
│       ├── server.go     # HTTP routes and static asset handler
│       ├── server_test.go# Unit tests for SSRF, HMAC, and shortener lifecycle
│       └── store.go      # Thread-safe link store with atomic JSON snapshots
├── public/
│   ├── index.html        # Web interface
│   ├── styles.css        # Stylesheet
│   ├── app.js            # Frontend logic and QR renderer
│   └── favicon.svg       # SVG icon
├── Dockerfile            # Container build
├── go.mod                # Go module definition
└── vercel.json           # Vercel routing configuration
```

## API Reference

### `POST /api/shorten`

Creates a shortened URL and returns both a short code link and a stateless signed link.

**Request body:**
```json
{
  "url": "https://github.com/GOG-777",
  "alias": "gog",
  "ttl": "7d"
}
```

- `url` (required): Destination `http://` or `https://` URL (max 2048 characters).
- `alias` (optional): Custom slug between 3 and 32 characters (`a-z`, `A-Z`, `0-9`, `-`, `_`).
- `ttl` (optional): Expiration preset (`1h`, `24h`, `7d`, `30d`, or `never`).

**Response (`201 Created`):**
```json
{
  "code": "gog",
  "shortUrl": "https://wrap.thgenius.xyz/gog",
  "statelessUrl": "https://wrap.thgenius.xyz/w/...",
  "targetUrl": "https://github.com/GOG-777",
  "createdAt": 1730000000,
  "expiresAt": 1730604800
}
```

### `GET /:code`

Redirects visitors (`302 Found`) to the destination URL associated with `:code` and increments click telemetry. Sends `Referrer-Policy: no-referrer` and `X-Robots-Tag: noindex, nofollow`.

### `GET /w/:token`

Verifies the HMAC-SHA256 signature and expiration timestamp of `:token`, decompresses the destination URL, validates it against SSRF rules, and redirects visitors (`302 Found`).

### `GET /api/stats?code=:code`

Returns click count, creation timestamp, expiration timestamp, last click time, and referrer host breakdown for a short code.

### `GET /api/health`

Returns service health, active link count, and server uptime.

## Configuration

- `PORT` (default `8080`): HTTP listen port for standalone server mode.
- `WRAP_SECRET`: Secret key used to sign and verify `/w/:token` links.
- `WRAP_BASE_URL`: Canonical base URL used when formatting returned links.
- `WRAP_STORE_FILE` (default `/tmp/wrap-links.json`): File path for JSON state snapshots.
- `WRAP_STATIC_DIR` (default `./public`): Path to static frontend assets.

## Running Locally

Start the standalone Go server:

```bash
go run ./cmd/wrap
```

Run the test suite:

```bash
go test ./pkg/wrap
```

## License

Released under the [MIT License](LICENSE).

```text
MIT License
Copyright (c) 2026 GOG (thgenius.xyz)
```
