# testserver

A tiny stdlib-only HTTP server that exercises every feature of **Yon**, plus
`testserver.yon` — a ready-made collection of 13 requests pointing at it.

## Run

From the repo root:

```sh
go run ./testserver          # listens on http://localhost:7878
```

Then in Yon: **File → Open** → `testserver/testserver.yon` (or `yon testserver/testserver.yon`).

## Endpoints

| Endpoint | Purpose |
|---|---|
| `/get` `/post` `/put` `/delete` | echo method, query, headers, body as JSON |
| `/headers` | echo request headers |
| `/basic-auth/{user}/{pass}` | requires Basic auth (`alice` / `secret`) |
| `/bearer` | requires `Authorization: Bearer yon-demo-token` |
| `/status/{code}` | returns that status code |
| `/redirect` | 302 → `/get` (tests follow-redirect) |
| `/large` | ~600 KB JSON (tests the 256 KB display truncation) |
| `/slow?seconds=N` | sleeps N s, honouring cancellation (tests Cancel / timeout) |
| `/json` | nested JSON sample (tests Pretty syntax colouring) |

## Demo credentials

- Bearer token: `yon-demo-token` (set as the collection-level default auth)
- Basic: `alice` / `secret`

## Tests

`main_test.go` mounts the routes on an `httptest` server and verifies every
endpoint (echo, Basic/Bearer auth, status codes, redirect, the >256 KB body, and
slow-endpoint cancellation) — run with `go test ./testserver`.
