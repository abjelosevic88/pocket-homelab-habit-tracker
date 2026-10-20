# API

Base path `/api/v1`. JSON in, JSON out. Authenticate with the session cookie (browser) or a personal token:

```
Authorization: Bearer ph_xxxxxxxx…
```

Create tokens under **Settings → API tokens**. Dates are `YYYY-MM-DD` in the server's `TZ`; months are `YYYY-MM`.

Every account has its own data. Whatever authenticates the request (session, `ph_` token, a Pocket Core token, a single sign-on session) resolves to one account, and every endpoint reads and writes only that account's habits, trackers, entries, journal, weather, locations and settings. Another account's ids answer `404`. Only the app title is shared by the box (admins change it).

### Pocket Core app links

With Pocket Core on the box, another Pocket app can act for a person with a five-minute `pcx_` token Core issues (`Authorization: Bearer pcx_…`). The token resolves to that person's account like any other, and only these routes are open to it:

| Scope | Name | Routes |
|---|---|---|
| (any link) | | `GET /auth/me` |
| `habits:read` | See your habits | `GET /habits`, `GET /habits/{id}`, `GET /days/{date}` |
| `entries:write` | Tick off habits | `PUT /habits/{id}/entries/{date}`, `DELETE /habits/{id}/entries/{date}` |

Anything else answers `403` ("this Pocket Core link does not cover …"). Events published for a change made through a link carry `"via": "<acting app slug>"`. Links are switched on and off in Core → **Links** (box-wide) and **Settings → Apps acting for me** (per person).

For a ready-made client, [Pocket Homelab](https://pockethomelab.com/) (iPhone, iPad, Mac, Android) uses this API with a token: the check-in, the month page, the dashboard, insights, and the habits and trackers. See the README's [Companion app](../README.md#companion-app-pocket-homelab) section.

## Day-to-day

| Method | Path | Notes |
|---|---|---|
| `GET` | `/today` | Everything for today: scheduled habits with status, trackers with value and previous value, journal text, done/total. |
| `GET` | `/days/{date}` | Same for any date. |
| `POST` | `/checkin` | Write a whole day at once (below). |
| `GET` | `/months/{month}` | The month page: days, habits with entries + counts + streak, trackers with entries + avg/min/max, journal, `important` (date → note), summary. `current` works as a month. |
| `GET` | `/dashboard` | Compact status for widgets. |
| `GET` | `/insights?months=3` or `?from=&to=` | Rates, streaks, weekday heat, tracker series, correlations, journal lines. |

### `POST /checkin`

```json
{
  "date": "2026-10-11",
  "habits":   { "1": 1, "2": -1, "5": 0 },
  "trackers": { "1": 4, "2": 3, "3": 81.2 },
  "journal":  "Long walk by the river."
}
```

- `habits`: habit id → `1` done, `-1` missed, `0` clear.
- `trackers`: tracker id → number, or `null` to clear. Yes/no trackers take `1` (yes) or `0` (no).
- `journal_bad`: the optional "one bad thing" line (Settings → General).
- `important`: the day's important note; the month page marks the day red.
- Every key is optional; `date` defaults to today. Future dates are rejected. Returns the day payload.

Get the ids from `GET /habits` and `GET /trackers`.

### Single cells

| Method | Path | Body |
|---|---|---|
| `PUT` | `/habits/{id}/entries/{date}` | `{"status": 1 \| -1 \| 0}` |
| `DELETE` | `/habits/{id}/entries/{date}` | |
| `PUT` | `/trackers/{id}/entries/{date}` | `{"value": 4.5}` or `{"value": null}` |
| `DELETE` | `/trackers/{id}/entries/{date}` | |
| `PUT` | `/journal/{date}` | `{"text": "…"}`, `{"bad": "…"}` and/or `{"important": "…"}`; a field left out is kept, all empty deletes the day. `important` may go on a day ahead, the lines may not. |
| `GET` | `/journal?from=&to=` | `{date: text}` |

## Habits & trackers

| Method | Path | Notes |
|---|---|---|
| `GET` | `/habits` | Active habits. `?all=1` includes ended, `?month=YYYY-MM` those on that page. |
| `POST` | `/habits` | `{"name","color","days":"1234567","start_date","end_date"}` — `days` are ISO weekdays. |
| `PUT` / `DELETE` | `/habits/{id}` | Update / delete (with all entries). |
| `POST` | `/habits/{id}/end` | Ends the habit at the end of the current month (`{"date": …}` to pick another month). |
| `POST` | `/habits/{id}/resume` | Clears the end date. |
| `PUT` | `/habits/reorder` | `{"ids":[3,1,2]}` |

Trackers mirror this under `/trackers`: `{"name","kind":"scale"|"number"|"yesno","min_value","max_value","step","unit","higher_is_better","start_date","end_date"}`.

## Settings, data, health

| Method | Path | Notes |
|---|---|---|
| `GET` / `PUT` | `/settings` | Title, week start, landing page, journal prompt, reminder. |
| `POST` | `/settings/test-reminder` | Sends a test using the posted settings. |
| `GET` | `/settings/system` | Version, runtime, db size, row counts. |
| `GET` / `POST` / `DELETE` | `/tokens`, `/tokens/{id}` | Personal tokens (plaintext shown once). |
| `GET` | `/backup` | Consistent SQLite copy (admin only, and only while nobody else has an account: 403 otherwise). |
| `GET` | `/export.json`, `/export.csv` | Everything. |
| `POST` | `/import` | Merge an export JSON (match by name, upsert entries). |
| `GET` | `/healthz`, `/readyz`, `/metrics` | Unauthenticated, outside `/api/v1`. |

## Recipes

### Homepage widget

```yaml
- Habits:
    href: https://habits.example.com/today
    icon: mdi-check-decagram
    widget:
      type: customapi
      url: http://pocket-habits:8080/api/v1/dashboard
      refreshInterval: 300000
      headers:
        Authorization: Bearer {{HOMEPAGE_VAR_HABITS_TOKEN}}
      mappings:
        - field: today_done
          label: Today
          format: text
          suffix: " done"
        - field: today_left
          label: Left
        - field: month_rate_pct
          label: Month
          suffix: "%"
        - field: perfect_streak
          label: Streak
          suffix: " days"
```

### iOS Shortcut

*Get Contents of URL* → `POST` → `https://habits.example.com/api/v1/checkin` → Headers `Authorization: Bearer …` → Request Body JSON with the ids you want. Ask for a number with *Ask for Input* and put it in `trackers`.

### Home Assistant

```yaml
rest_command:
  habit_done:
    url: https://habits.example.com/api/v1/checkin
    method: POST
    headers: { Authorization: "Bearer !secret habits_token", Content-Type: application/json }
    payload: '{"habits": {"{{ habit }}": 1}}'
```

### Prometheus

`/metrics` exposes `pocket_habits_today_done`, `pocket_habits_today_total`, `pocket_habits_month_done`, `pocket_habits_month_missed`, `pocket_habits_perfect_streak`, request counters and row counts.

## Weather & location

The server fetches the day's weather from [Open-Meteo](https://open-meteo.com/) (free, no key) when **Settings → Weather** is on: the condition (`clear`, `partly`, `cloudy`, `fog`, `rain`, `snow`, `storm`), high and low temperature, precipitation, sunshine and daylight hours. The home location is the fallback; a day with a reported location (below) is fetched for that place instead. The scheduler refreshes today and yesterday every three hours and backfills missing days since the first habit (two years at most). The day, month, insights and dashboard answers carry a `weather` field.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/weather?from=&to=` | The stored days: `[{date, code, condition, t_max, t_min, precip_mm, sunshine_h, daylight_h, overridden, lat, lon, place, fetched_at}]`. |
| `PUT` | `/weather/{date}` | `{"condition": "rain"}` sets a day's condition by hand; `""` puts the fetched one back. |
| `POST` | `/weather/refresh` | `{"from","to"}` fetches a range now; an empty body refreshes today and yesterday and backfills. Returns `{"days": n}`. |
| `GET` | `/weather/geocode?q=` | Place lookup for the home location: `[{name, label, lat, lon, timezone}]`. |
| `GET` | `/location?from=&to=` | The days with a reported location. |
| `POST` | `/location` | `{"date": "2026-10-11", "lat": 47.37, "lon": 8.54, "place": "Zürich", "source": "iphone"}` — where you were that day (`date` defaults to today). The day's weather is fetched for that place at once and returned as `weather`. Pocket Homelab sends this once a day when it is opened; a Shortcut or Home Assistant can too. |

Insights add `weather: {days, by_condition, by_temperature, by_daylight, units}`, each bucket `{key, label, days, scores: [{tracker_id, tracker_name, avg, n}], habit_rate, done, missed}`.
| | | `default_value` (optional): assumed on every active past day with no entry — `0`/`1` for yes/no, a step for scales, any number for measurements; `null` = none. Month/day/insights return these days filled in, flagged via `defaulted`. |
