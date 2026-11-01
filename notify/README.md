# B-Wise Notify

Central delivery outbox for service-token callers. User-owned Web Push subscriptions; no standalone dashboard menu.

## Contract

- `POST /api/internal/notifications`: service token, JSON `channel`, `recipient` (or push `user_id`), `topic`, `title`, `body`, optional JSON-string `payload`.
- `Idempotency-Key`: stable domain outbox ID; scoped by authenticated service. Replay returns the existing row, including final delivery status.
- `GET /api/internal/notifications/:id`: delivery status. HTTP 201 means accepted, **not delivered**.
- `GET /api/push/config`: authenticated user; `{success,data:{enabled,public_key}}`.
- `POST /api/push/subscriptions`: authenticated user, standard browser subscription JSON + optional `user_agent`.
- `DELETE /api/push/subscriptions`: authenticated owner only, JSON `endpoint`.

Statuses: `pending`, `sent` (push provider accepted), `failed`, `skipped` (no active push subscriptions). `sent` does not prove the person saw a notification. Retry: 10 minutes × attempts, max 5; partial successes are recorded per subscription so other-device retries do not resend successful ones. Provider 404/410 removes a subscription; 401/403/429 retain it. Single worker replica supported; crash between external provider acceptance and DB write can still cause at-least-once delivery. SW `tag` collapses duplicate notifications.

## Configuration

Standard scaffold DB/SSO settings plus:

- `PUSH_VAPID_PUBLIC`, `PUSH_VAPID_PRIVATE`: generate once with `go run ./cmd/gen-vapid > /secure/path/vapid.env` (0600). Preserve across deployments; never commit or log keys.
- `PUSH_VAPID_SUBJECT=mailto:it@binawan.ac.id`
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM`
- `WA_WEBHOOK_URL`, `WA_WEBHOOK_TOKEN`: webhook accepts `user_id` for IKU user routing or `to` for explicit phone routing.
- Consumers Helpdesk/IKU: `NOTIFY_URL=http://127.0.0.1:8092`, their own existing `SSO_URL` and `SERVICE_CLIENT_*`.

Without NOTIFY_URL, consumers retain their old transport. With it, new domain push rows are relayed idempotently and remain pending until central delivery settles. Existing email/WA rows are relayed rather than sent twice. IKU email has no trustworthy recipient-profile mapping yet: it fails explicitly instead of fabricating user-ID addresses. SMTP/WA still require actual channel configuration.

Portal: same-origin nginx `/api/notify/` → notify `/`; optional direct dev override `NEXT_PUBLIC_NOTIFY_URL`. Enable/disable under Profile, only on explicit user interaction. On iOS, install to Home Screen first. Push content on lock screens is deliberately generic; detail requires authenticated navigation. Browser logout unsubscribes locally.

## Verification

`go test ./...` runs unit tests. To exercise actual PostgreSQL constraints/retry/ownership in an isolated temporary schema:

```
NOTIFY_TEST_DSN='host=localhost user=macbook dbname=postgres sslmode=disable' go test ./internal/service/notifsvc
```

Tests use encrypted Web Push requests with an in-process fake HTTP transport: no external notifications. Browser-provider receipt remains a separate manual E2E check on an opted-in device.

## Deployment / rollback

Commit/push to GitHub, server fast-forward pull, build fresh binaries, back up existing binary + .env, atomically rename and restart notify before consumers. Do not regenerate VAPID keys on redeploy. To disable central relay, restore consumer .env without NOTIFY_URL and restart; pending push rows are left intact for later resumption. Existing in-app notifications continue unaffected. Keep old binaries for rollback. AI is separate: staging must use AI_DISABLED=1 until a dedicated reachable model service exists.
