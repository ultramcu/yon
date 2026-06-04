# Yon

Yon is an offline desktop client for testing HTTP APIs. This glossary fixes the
language used across the model, engine, and UI so the same words mean the same
things everywhere.

## Language

**Collection**:
A saved set of Requests, persisted as one human-readable `.yon` file.
_Avoid_: workspace, project, file (the file is the storage, not the concept).

**Folder**:
A named, one-level grouping of Requests inside a Collection (display grouping;
Requests stay a flat list underneath).
_Avoid_: group, directory.

**Request**:
A saved HTTP request definition (method, URL, params, headers, auth, body) in a
Collection — a reusable template, not a single send.
_Avoid_: call, endpoint.

**Response**:
The result of sending a Request (status, headers, body, timing/size). Read-only.

**Environment**:
A named set of Variables (e.g. Local / Staging / Prod); exactly one is active per
Collection and its Variables take precedence over Collection-level ones.

**Variable**:
A named `{{template}}` value substituted into a Request's text fields when it is
sent under the active Environment.

**Secret**:
A Variable whose value is kept out of the committed file and stored in a
gitignored `.env` sibling instead.
_Avoid_: credential, password (those are kinds of secret values, not the concept).

**SSH jump host**:
The *configuration* on an Environment naming an SSH server (host, port, user,
auth) that its Requests should be dialed *through*. A jump host is settings, not a
live connection.
_Avoid_: bastion, proxy, port-forward.

**Tunnel**:
The *live*, in-process SSH connection Yon holds open to a jump host. Requests are
dialed through it, and it is what the tunnel-status view reports as active. Yon
maintains the connection itself (via `golang.org/x/crypto/ssh`); it does NOT open
a local forwarded port or shell out to the system `ssh` binary.
_Avoid_: using "tunnel" for the config (that is the **jump host**).

## Relationships

- A **Collection** contains many **Requests**, optionally organized into **Folders**.
- A **Collection** has many **Environments**; one is active at a time.
- An **Environment** has many **Variables**; a **Variable** may be a **Secret**.
- An **Environment** may define one **SSH jump host**; when that Environment is
  active, Yon holds open one **Tunnel** to it and dials every **Request** through it.

## Example dialogue

> **Dev:** "If I'm on the Staging **Environment** and send a **Request** to
> `https://orders.internal:8443`, does Yon rewrite the URL?"
> **Maintainer:** "No. Staging has an **SSH jump host**, so Yon dials
> `orders.internal:8443` *through* the jump host — the URL stays exactly as typed."

## Flagged ambiguities

- "tunnel" was used for both the config and the connection — resolved into two
  terms: **SSH jump host** (the config) vs **Tunnel** (the live in-process
  connection). The mechanism is dial-through, NOT a local port-forward or SOCKS proxy,
  and NOT a shelled-out system `ssh` process.
