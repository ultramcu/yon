<p align="center">
  <img src="assets/logo/yon-icon-512.png" alt="Yon" width="128" height="128">
</p>

<h1 align="center">Yon</h1>

<p align="center"><b>Throw a request. Catch a response.</b></p>

<p align="center">
A minimal, fast, offline desktop client for testing HTTP APIs — a lightweight
Postman alternative. No account, no cloud, no telemetry. One native binary for
macOS, Windows, and Linux.
</p>

---

## Why Yon

Postman keeps growing heavier and more account-gated. Yon is the opposite: a tiny
native app that opens instantly, never asks you to log in, and keeps your work in
plain files on your own disk. It does the everyday job — fire a request, read the
response — and gets out of your way.

The name is Thai: **โยน (yon)** = *to throw*. You throw a request; you catch a response.

## Features (v1)

- **Methods:** GET · POST · PUT · DELETE
- **Query parameters** and **headers** as toggleable key/value tables
- **Auth:** None · Basic · Bearer — set a default on the collection and inherit or
  override it per request
- **Body:** None · JSON (auto `Content-Type` + pretty) · Text
- **Response viewer:** status · time · size · headers, with a **Pretty** (indented,
  JSON syntax-coloured) / **Raw** toggle; large bodies are capped on screen with a
  *Save to file* escape hatch
- **Send / Cancel** any in-flight request
- **Collections** saved as plain-JSON `.yon` files — one window per collection, one
  tab per request; your open collections and tabs are restored on the next launch
- **Connection settings:** request timeout, follow redirects, and an *Allow insecure
  TLS* toggle for self-signed dev APIs

## Install / run from source

Requires **Go 1.26+** and a C toolchain (Fyne uses cgo/OpenGL).

```sh
git clone https://github.com/ultramcu/yon
cd yon
go run .
```

Build a binary:

```sh
go build -o yon .
```

Open a collection on launch by passing its `.yon` file (one window per file; this
takes precedence over restoring the previous session):

```sh
yon mycollection.yon
yon api/users.yon api/billing.yon
```

Package a native app bundle with an embedded icon (optional, needs the Fyne CLI):

```sh
go run fyne.io/fyne/v2/cmd/fyne package -os darwin   # or windows / linux
```

## The `.yon` file

A collection is one human-readable JSON file you can keep in git:

```json
{
  "version": 1,
  "name": "My API",
  "auth": { "kind": "bearer", "token": "..." },
  "requests": [
    {
      "name": "List users",
      "method": "GET",
      "url": "https://api.example.com/users",
      "params": [{ "key": "page", "value": "1", "enabled": true }],
      "auth": { "kind": "inherit" }
    }
  ]
}
```

## Architecture

The core is UI-free and fully testable; Fyne lives only in the front end.

| Package | Role |
|---|---|
| `internal/model` | data types + auth resolution (no Fyne, no net) |
| `internal/yonner` | builds and sends the HTTP request (the engine) |
| `internal/store` | reads/writes `.yon` files |
| `internal/ui` + `main.go` | the Fyne desktop UI (the only Fyne importers) |

See [`CONTEXT.md`](CONTEXT.md) for the glossary, [`SCOPE.md`](SCOPE.md) for the v1
scope, and [`docs/adr/`](docs/adr) for the key decisions.

## Roadmap (post-v1)

Environments & variables · request history · collection folders · import from
Postman / OpenAPI / `.http` · form-data & multipart bodies · copy-as-curl ·
pre-request scripts.

## License

MIT
