# Configuration

All settings are environment variables. Nothing is required.

## Server

| Variable | Default | Description |
|---|---|---|
| `HOST` | `0.0.0.0` | Listen address. |
| `PORT` | `8080` | Listen port. |
| `BASE_URL` | – | Public URL, used in reminder "click" links (e.g. `https://habits.example.com`). |
| `DATA_DIR` | `./data` (`/data` in the container) | Database and backups. |
| `DB_PATH` | `$DATA_DIR/pocket-habits.db` | Override the database file. |
| `TZ` | `UTC` | Timezone for "today", month pages and the reminder. The container ships `tzdata`. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | `text` | `text` or `json`. |
| `TRUST_PROXY` | `true` | Honour `X-Forwarded-For` / `X-Real-IP` for logging. |
| `METRICS_ENABLED` | `true` | Serve Prometheus metrics on `/metrics` (unauthenticated — restrict at the proxy if exposed). |
| `SCHEDULER_ENABLED` | `true` | Run the once-a-minute reminder scheduler. |
| `DEMO_DATA` | `false` | Seed seven habits and ~70 days of history into an *empty* database. |

## Authentication

| Variable | Default | Description |
|---|---|---|
| `AUTH_MODE` | `local` | `local`: built-in email + password with a first-run setup screen. `proxy`: trust the user name from a header set by your SSO proxy. `none`: no login at all, one implicit user — only for trusted networks. |
| `AUTH_PROXY_HEADER` | `Remote-User` | Header carrying the user name in `proxy` mode. |
| `AUTH_PROXY_EMAIL_HEADER` | `Remote-Email` | Header carrying the email in `proxy` mode. |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | In `local` mode, create this user on first start instead of showing the setup screen. Password must be 8+ characters. |
| `SESSION_TTL` | `720h` | Browser session lifetime. |
| `SECURE_COOKIES` | `false` | Mark the session cookie `Secure`. Turn on behind HTTPS. |

API tokens (`Authorization: Bearer …` or `X-API-Key`) work in every mode.

## Reminder

Configured in the UI under **Settings → Reminder**, not by environment: time of day, endpoint URL and format (`ntfy`, `apprise`, `json`). The scheduler checks once a minute, fires at most once per day, and only if habits are unmarked or the journal is empty.

## Forgot your password?

```bash
docker exec pocket-habits pocket-habits reset-password            # first user, random password printed
docker exec pocket-habits pocket-habits reset-password -email me@example.com -password NewPass123
```
