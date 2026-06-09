---
name: context7-auth
description: >-
  Use when auditing, debugging, or fixing Context7/ctx7 CLI authentication,
  device-code login, token refresh, Context7 API headers, or compatibility gaps
  between this ctx repo and the official upstash/context7 CLI. Trigger on
  Context7 refresh failures such as HTTP 400, invalid_grant, stale ctx7 tokens,
  ctx auth login/status issues, or requests to compare current official
  Context7 CLI auth behavior with local ctx code.
---

# Context7 Auth Alignment

Use this skill to align local `ctx` auth behavior with the official Context7
CLI instead of guessing from memory. Treat upstream as live and version-sensitive.

## Source Of Truth

Start from the current upstream CLI source tree before proposing a fix:

```bash
ctx read github://upstash/context7/packages/cli/src 2>&1
```

Use the directory listing to follow current code shape. Look for names and
symbols like `auth`, `oauth`, `device`, `token`, `refresh`, `client_id`,
`credentials`, `constants`, `api`, `login`, and `whoami`; then read only the
specific files that currently own those paths. Do not assume the old file names,
flow shape, or locations still exist.

If GitHub reads are flaky or several files are needed, shallow clone into
`.scratch/`:

```bash
mkdir -p .scratch
git clone --depth 1 https://github.com/upstash/context7.git .scratch/context7
rg -n "auth|oauth|device|token|refresh|client_id|credentials|whoami" .scratch/context7/packages/cli/src
```

Do not preserve `.scratch/` as part of the fix.

## Compare Checklist

Compare upstream against the local auth surfaces:

- `api/auth.go`: token shape, device authorization start request, device-token
  polling behavior, refresh request, refresh error handling, credential save
  semantics.
- `api/client.go`: Context7 API base URL, auth injection, Context7 headers.
- `cmd/auth.go`: login/status/logout UX and whether status accidentally retries
  stale refresh on every run.
- `config/credentials.go`: storage path, file permissions, migration from old
  token files, and whether updates can silently drop unrelated credentials.

Known upstream anchors to re-check every time:

- OAuth token endpoint: `/api/oauth/token`.
- CLI client id currently lives in `packages/cli/src/constants.ts` as
  `CLI_CLIENT_ID`; do not hard-code from memory without re-reading upstream.
- Refresh grant body should include `grant_type=refresh_token`, `client_id`,
  and `refresh_token`.
- Login currently uses OAuth device authorization, not local PKCE callback:
  - start request: `POST /api/oauth/device/code` with `client_id` and best-effort
    `hostname`.
  - poll request: `POST /api/oauth/device/token` with
    `grant_type=urn:ietf:params:oauth:grant-type:device_code`, `client_id`, and
    `device_code`.
  - `authorization_pending` keeps polling; `slow_down`, transient network
    errors, and 5xx responses increase the polling interval by 5 seconds.
- Old PKCE/callback code (`/api/oauth/authorize`, `authorization_code`,
  `code_verifier`, fixed localhost callback ports) is legacy drift unless
  upstream has explicitly reintroduced it. Do not preserve or revive it by
  default.
- Official request headers may include client identity/version fields; align
  only when they matter for server behavior or diagnostics.

## Failure Pattern

For `Context7 token refresh failed: refresh failed: HTTP 400`, first determine
which class it belongs to:

1. Local request drift: missing or stale `client_id`, wrong endpoint, stale
   device-flow semantics, or old installed binary.
2. Expired/revoked refresh token: upstream returns `invalid_grant`; the CLI
   cannot repair this without a new login.
3. Poor diagnostics: local code hides the JSON `error` / `error_description`,
   so the user sees only `HTTP 400`.
4. Credential write bug: refresh succeeds but local save drops a still-valid
   refresh token when the server returns only a new access token.

Fix the confirmed class. Do not rewrite the whole auth layer if a two-line
protocol drift is the cause.

For login failures, first separate device-flow errors from refresh errors:

1. Device authorization start failed: inspect `/api/oauth/device/code` status
   and OAuth error JSON.
2. Polling failed: confirm handling for `authorization_pending`, `slow_down`,
   `access_denied`, `expired_token`, 5xx, and network errors.
3. Browser-open failure: this should not invalidate the device code; the CLI
   must still print the verification URL/code for manual completion.
4. Credential save failure: approval succeeded but `SaveTokens` failed; do not
   misdiagnose this as an OAuth rejection.

## Repair Defaults

- Parse OAuth error JSON and include `error_description` in diagnostics.
- Clear stored Context7 OAuth credentials only for terminal token errors such as
  `invalid_grant`; do not delete credentials for transient network failures.
- Preserve the old refresh token when a successful refresh response omits a new
  `refresh_token`.
- Keep the device authorization flow aligned with upstream. Delete local
  PKCE/callback machinery unless upstream brings it back.
- Treat `slow_down`, transient network failures, and 5xx device-token polling
  responses as retryable with a +5s poll backoff.
- Keep anonymous fallback for `search`/`docs` if existing behavior depends on
  unauthenticated access, but stop repeated stale-token retries when possible.
- If the installed `ctx` binary is older than the repo fix, call that out
  directly; source changes alone do not update the user's executable.

## Validation

Run focused tests first, then the full suite:

```bash
go test ./api
go test ./...
```

For a live smoke test, use a command that previously triggered refresh:

```bash
go run . search notion "ntn CLI" 2>&1
```

Do not print access tokens or refresh tokens. If probing the token endpoint
manually, print only HTTP status and OAuth error fields.
