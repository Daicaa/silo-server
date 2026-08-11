# PR screenshots — real captures from locally-built branches

Every image here was captured with headless Chromium (Playwright, 1440x900 viewport)
against a locally built and running Silo server binary of the exact branch commit,
backed by a fresh disposable Postgres (`pgvector/pgvector:pg17`) and `redis:alpine`.
Nothing is mocked or edited; the UI state was produced through the app's own
API/UI flows. Dark theme is the app default — no styling changes were made.

Server run (per branch): built the SPA with `pnpm install && pnpm run build` in
`web/` (the Go binary embeds `web/dist`), then `go build ./cmd/silo`, then ran the
binary with `DATABASE_URL=postgres://silo:silo@127.0.0.1:15432/silo?sslmode=disable`,
`REDIS_URL=redis://127.0.0.1:16379`, `SECRET_KEY=<throwaway>`, `PORT=18080`,
`JF_PORT=18096`, `MODE=integrated`. First-run setup, profiles, and devices were
created through the public API (`/api/v1/auth/setup`, `/api/v1/profiles`,
device-registering `PUT /api/v1/settings/device/...` calls carrying
`X-Silo-Device-Id/-Name/-Platform` headers).

## PR: feat(devices): user-set custom device names
Branch `feat/device-custom-names`, commit `39b486e`.

- **rename-01-device-list.png** — Settings → Devices ("Your devices") for the
  primary profile. The list shows the before/after story in one frame: two
  still-duplicate "Apple TV" rows and an "iPhone", plus the renamed device
  "Living Room" (selected). The detail header shows the new meta line
  `reports as "Apple TV"`, proving the reported name is preserved alongside the
  custom name. The rename itself was performed via the new
  `PATCH /api/v1/devices/{device_id}` endpoint with `{"name":"Living Room"}`.
- **rename-02-detail-rename.png** — the same device with the rename affordance
  open: the inline input's placeholder is the reported name ("Apple TV" —
  visible because the input was cleared, which is also how a user reverts to the
  reported name), with Save/Cancel and the `reports as "Apple TV"` meta line
  still visible below.

## PR: feat(devices): opt-in setting restricting device forget to the primary profile
Branch `feat/primary-only-device-forget`, commit `165bb33`.

- **forget-01-admin-toggle.png** — Admin → Settings → General (full page): the
  new "Devices" group with the "Only Primary Profile Can Forget Devices" toggle
  ON. The state shown is persisted — it was toggled, saved via the Save bar, and
  the page reloaded before capture (backing key
  `devices.forget_requires_primary`).
- **forget-02-denied-toast.png** — logged in as **Riley**, a non-primary profile
  of a regular (non-admin) user, with the setting ON. Riley pressed Forget on
  their **own** device (iPad) and confirmed; the server answered 403 and the UI
  shows the toast: "Forgetting a device on this server requires the primary
  profile or admin access".

Captured 2026-08-11 on a throwaway instance at 127.0.0.1:18080 (containers
`shot-pg`/`shot-redis`, removed after capture). No production data or services
were involved.
