# OpenSearchGateway

OpenSearchGateway is a small Go web server that sits in front of an OpenSearch cluster and does two jobs:

- it turns simple JSON HTTP writes into rollover-friendly OpenSearch documents
- it fronts OpenSearch Dashboards with an LDAP-backed login flow and per-user OpenSearch provisioning

It is designed for a very specific ingestion model:

- clients send JSON to `POST /ingest/<index>`
- every document must contain a top-level `event_time`
- the gateway derives a daily write alias from that timestamp
- OpenSearch writes go through rollover aliases and backing indices
- OpenSearch Dashboards gets a per-index tenant and a matching data view automatically
- LDAP users can sign in through the gateway and reach Dashboards without exposing the admin account

In short, this project gives you a thin HTTP ingest layer and a thin web access layer in front of OpenSearch, with just enough bootstrap logic to make daily rollover-based indexing, tenant-aware Dashboards discovery, and LDAP-backed Dashboards access work without manual setup for each new index family.

## What It Does

When the gateway starts, it bootstraps shared cluster resources:

- ensures the ISM policy `generic-rollover-100m` exists
- ensures a shared index template exists for rollover backing indices
- starts an HTTP server on `LISTEN_ADDR` (default `:8080`)

When a user signs in through `POST /login`, the gateway:

1. validates the submitted username and password against LDAP
2. derives namespace access from LDAP groups like `<namespace>_r`, `<namespace>_rw`, and `<namespace>_rwd`
3. ensures a matching OpenSearch Security role for each namespace
4. ensures a tenant and data view exist for each namespace
5. creates or replaces the internal OpenSearch user with the same username and password semantics
6. stores a short-lived server-side session
7. reverse proxies OpenSearch Dashboards under `/dashboards` using the logged-in user's basic auth

When a client sends a document to `POST /ingest/<index>`, the gateway:

1. validates the path and index name
2. requires `Content-Type: application/json`
3. parses the body as a single JSON object
4. requires top-level `event_time` as a UTC RFC3339 string ending in `Z`
5. derives the write alias as:

```text
<index>-YYYYMMDD-rollover
```

6. ensures an OpenSearch Security tenant named exactly `<index>`
7. ensures an OpenSearch Dashboards data view inside that tenant with pattern:

```text
<index>-*
```

8. checks whether the daily write alias exists
9. if missing, creates the first backing index:

```text
<index>-YYYYMMDD-rollover-000001
```

10. attaches the rollover alias and ISM policy
11. indexes the document through the alias
12. returns a compact JSON response describing the write

## Why This Exists

This gateway is useful when you want:

- a very simple HTTP ingest interface instead of exposing OpenSearch directly
- deterministic daily alias naming based on an event timestamp
- rollover-compatible index creation
- automatic Dashboards setup for each logical index family
- LDAP-backed sign-in for Dashboards without giving end users the admin credential
- namespace-scoped OpenSearch users and Dashboards tenants derived from LDAP groups
- a lightweight developer stack you can run locally with Docker Compose

It is especially handy for internal tools, demos, prototypes, and ingestion pipelines where producers should not need to know OpenSearch index template, alias, tenant, or Dashboards setup details.

## HTTP Interface

### `GET /`

Redirects to `/login`.

### `GET /login`

Serves the gateway login page.

### `POST /login`

Authenticates the submitted LDAP credentials, provisions the user's OpenSearch roles and internal user, creates a gateway session, and redirects to `/dashboards/` on success.

Login responses:

- `303` on success
- `401` for invalid username or password
- `403` when LDAP auth succeeds but no authorized namespace groups are present, or when the target OpenSearch internal user is reserved/hidden
- `502` when LDAP, OpenSearch Security API, or Dashboards provisioning fails

### `POST /logout`

Clears the gateway session and redirects back to `/login`.

### `GET /dashboards`
### `GET /dashboards/*`

Reverse proxies OpenSearch Dashboards through the gateway. These routes require a valid gateway session. The gateway injects the logged-in user's basic auth header on proxied requests and refreshes the session idle timeout while Dashboards is in use.

### `GET /demo`

Serves a small demo page where you can:

- enter an index name
- paste a JSON document
- submit it directly to the gateway from the browser

### `POST /ingest/<index>`

Primary ingest endpoint.

Accepted path examples:

```text
/ingest/orders
/ingest/orders/
```

Rejected path examples:

```text
/ingest/
/ingest/orders/extra
/ingest/Orders
```

Index names must:

- start with a lowercase letter or digit
- only contain lowercase letters, digits, `-`, and `_`

The generated alias and first backing index must also fit within OpenSearch index naming limits.

### Required document shape

The request body must be a single JSON object with a top-level `event_time`.

Example:

```json
{
  "event_time": "2024-12-30T10:11:12Z",
  "message": "hello",
  "customer_id": 42,
  "status": "received"
}
```

Rules for `event_time`:

- must be present
- must be a string
- must be valid RFC3339
- must be UTC and end in `Z`

The gateway preserves the rest of the JSON body and normalizes `event_time` back into canonical UTC RFC3339 before indexing.

### Success response

Successful writes return `201 Created`.

Example:

```json
{
  "result": "created",
  "write_alias": "orders-20241230-rollover",
  "document_id": "abc123",
  "bootstrapped": true
}
```

### Error behavior

- `400` for request validation errors
- `405` for wrong HTTP methods
- `415` for non-JSON requests
- `502` when OpenSearch or Dashboards setup/indexing fails

The gateway is strict about Dashboards setup for indexed families. If tenant or data-view creation fails, the request fails before any document is written.

## OpenSearch Naming Model

For an ingest request like:

```text
POST /ingest/orders
```

with:

```json
{
  "event_time": "2024-12-30T10:11:12Z"
}
```

the gateway produces:

- tenant: `orders`
- Dashboards data view pattern: `orders-*`
- write alias: `orders-20241230-rollover`
- first backing index: `orders-20241230-rollover-000001`

Writes always go through the alias, not directly to the backing index.

## Dashboards And LDAP Behavior

For every new index family, the gateway creates:

- an OpenSearch Security tenant named exactly after the index family
- an OpenSearch Dashboards data view inside that tenant

For every successful LDAP login, the gateway also:

- derives namespace access from LDAP group names
- ensures one custom OpenSearch role per effective namespace access mode
- creates or replaces the corresponding internal OpenSearch user
- grants the built-in `kibana_user` role plus the namespace roles

That means:

- `orders` data goes with the `orders` tenant
- the Dashboards data view is created with title `orders-*`
- the time field is set to `event_time`
- a user with LDAP groups `team1_rwd` and `team2_rw` gets roles that map to `team1-*` and `team2-*`

This keeps data-view organization aligned with the ingest namespace.

## Local Development Stack

The repository includes a full local stack in [docker-compose.yml](/home/define42/git/OpenSearchGateway/docker-compose.yml):

- OpenSearch
- OpenSearch Dashboards
- OpenSearchGateway
- GLAuth LDAP

### Start the stack

```bash
docker compose up --build
```

Or with the included [makefile](/home/define42/git/OpenSearchGateway/makefile):

```bash
make
```

The compose stack exposes:

- OpenSearch: `https://localhost:9200`
- OpenSearch Dashboards: `http://localhost:5601`
- OpenSearchGateway: `http://localhost:8080`
- LDAP: `ldaps://localhost:389`

Default admin password in the local stack:

```text
Cedar7!FluxOrbit29
```

You can override it with:

```bash
export OPENSEARCH_ADMIN_PASSWORD='your-strong-password'
docker compose up --build
```

The bundled LDAP config includes a demo user you can use against the gateway login page:

```text
username: testuser
password: dogood
```

That user resolves to these namespace groups in the sample LDAP config:

- `team1_rwd`
- `team2_rw`
- `team10_r`

So after login, the gateway provisions access for `team1-*`, `team2-*`, and `team10-*`.

## Running the Gateway Without Docker

Run it directly with Go:

```bash
go run .
```

Useful environment variables:

- `LISTEN_ADDR`
- `OPENSEARCH_URL`
- `OPENSEARCH_USERNAME`
- `OPENSEARCH_PASSWORD`
- `DASHBOARDS_URL`
- `DASHBOARDS_USERNAME`
- `DASHBOARDS_PASSWORD`
- `DASHBOARDS_TENANT`
- `LDAP_URL`
- `LDAP_BASE_DN`
- `LDAP_USER_FILTER`
- `LDAP_GROUP_ATTRIBUTE`
- `LDAP_GROUP_PREFIX`
- `LDAP_USER_DOMAIN`
- `LDAP_STARTTLS`
- `LDAP_SKIP_TLS_VERIFY`

Current defaults in the code:

- `LISTEN_ADDR=:8080`
- `OPENSEARCH_URL=https://localhost:9200`
- `DASHBOARDS_URL=http://localhost:5601`
- username defaults to `admin`

Note that per-index data views are created in tenants named after the index, so `DASHBOARDS_TENANT` is not used for those auto-created views. It remains available as the default tenant value for generic Dashboards requests.

## Route Summary

- `GET /` redirects to `/login`
- `GET /login` renders the login form
- `POST /login` authenticates against LDAP and opens a Dashboards session
- `POST /logout` clears the session
- `GET /dashboards/*` proxies OpenSearch Dashboards for authenticated users
- `GET /demo` serves the browser demo ingest form
- `POST /ingest/<index>` ingests a JSON document into the rollover alias for that index family

## Example Ingest

```bash
curl -X POST http://localhost:8080/ingest/orders \
  -H 'Content-Type: application/json' \
  -d '{
    "event_time": "2024-12-30T10:11:12Z",
    "message": "order received",
    "customer_id": 42
  }'
```

## Project Files

- [main.go](/home/define42/git/OpenSearchGateway/main.go): gateway server, bootstrap logic, OpenSearch client helpers, demo page
- [main_test.go](/home/define42/git/OpenSearchGateway/main_test.go): request flow and bootstrap tests
- [docker-compose.yml](/home/define42/git/OpenSearchGateway/docker-compose.yml): local OpenSearch, Dashboards, and gateway stack
- [Dockerfile](/home/define42/git/OpenSearchGateway/Dockerfile): container image for the gateway
- [makefile](/home/define42/git/OpenSearchGateway/makefile): local compose convenience target

## Development Notes

- the HTTP client disables TLS verification for OpenSearch, which is convenient for local development but not production-safe
- the gateway does not include authentication or authorization on its ingest endpoint
- batching is not implemented; each request indexes one JSON document
- the index template and ISM policy are shared, global bootstrap resources
- the tenant and data-view setup is demand-driven and happens per index family on first ingest

## Testing

Run the test suite with:

```bash
go test ./...
```

## Summary

This project is a narrow, purpose-built ingest gateway for OpenSearch. It accepts JSON over HTTP, validates and routes documents by `event_time`, creates rollover-friendly daily aliases and backing indices, and automatically prepares matching Dashboards tenants and data views so newly ingested data is easier to discover.
