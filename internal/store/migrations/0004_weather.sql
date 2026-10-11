-- The day's weather, fetched from Open-Meteo for the home location or for where the phone
-- was that day; the condition can be overridden by hand. And the day's location, sent by
-- the companion app once a day.
CREATE TABLE weather_days (
  date         TEXT PRIMARY KEY,
  code         INTEGER NOT NULL DEFAULT -1,          -- WMO weather code, -1 = unknown
  condition    TEXT NOT NULL DEFAULT '',             -- clear | partly | cloudy | fog | rain | snow | storm
  t_max        REAL,
  t_min        REAL,
  precip_mm    REAL,
  sunshine_h   REAL,
  daylight_h   REAL,
  overridden   INTEGER NOT NULL DEFAULT 0,           -- 1 when the condition was set by hand
  lat          REAL NOT NULL DEFAULT 0,
  lon          REAL NOT NULL DEFAULT 0,
  place        TEXT NOT NULL DEFAULT '',
  fetched_at   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE location_days (
  date         TEXT PRIMARY KEY,
  lat          REAL NOT NULL,
  lon          REAL NOT NULL,
  place        TEXT NOT NULL DEFAULT '',
  source       TEXT NOT NULL DEFAULT '',             -- e.g. "iphone", "android", "mac"
  recorded_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
