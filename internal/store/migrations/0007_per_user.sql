-- Every row belongs to one person. Until now the content tables had no owner, so every
-- signed-in account shared one dataset. Existing rows go to the owner: the first admin, the
-- account that set the box up (members never had a way to own anything). Runs in one
-- transaction and is recorded in schema_migrations, so it is applied exactly once; the
-- backfills only touch rows that still have no owner.
CREATE TEMP TABLE _owner AS
  SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1;

-- habits / trackers and their day entries: an owner column, backfilled to the owner
-- (entries take their parent's owner).
ALTER TABLE habits ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE trackers ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE habit_entries ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE tracker_entries ADD COLUMN user_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
UPDATE habits SET user_id = (SELECT id FROM _owner) WHERE user_id IS NULL;
UPDATE trackers SET user_id = (SELECT id FROM _owner) WHERE user_id IS NULL;
UPDATE habit_entries SET user_id = (SELECT h.user_id FROM habits h WHERE h.id = habit_entries.habit_id) WHERE user_id IS NULL;
UPDATE tracker_entries SET user_id = (SELECT t.user_id FROM trackers t WHERE t.id = tracker_entries.tracker_id) WHERE user_id IS NULL;
CREATE INDEX IF NOT EXISTS habits_user ON habits(user_id, sort_order);
CREATE INDEX IF NOT EXISTS trackers_user ON trackers(user_id, sort_order);
CREATE INDEX IF NOT EXISTS habit_entries_user_date ON habit_entries(user_id, date);
CREATE INDEX IF NOT EXISTS tracker_entries_user_date ON tracker_entries(user_id, date);

-- journal, weather and location were keyed on the date alone: one row per day for the whole
-- box. Rebuilt keyed on (user_id, date). Nothing references these tables, so the rebuild
-- cascades nowhere.
CREATE TABLE journal_new (
  user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  text       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  bad_text   TEXT NOT NULL DEFAULT '',
  important  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, date)
);
INSERT INTO journal_new (user_id, date, text, updated_at, bad_text, important)
  SELECT (SELECT id FROM _owner), date, text, updated_at, bad_text, important FROM journal;
DROP TABLE journal;
ALTER TABLE journal_new RENAME TO journal;

CREATE TABLE weather_days_new (
  user_id      INTEGER REFERENCES users(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  code         INTEGER NOT NULL DEFAULT -1,
  condition    TEXT NOT NULL DEFAULT '',
  t_max        REAL,
  t_min        REAL,
  precip_mm    REAL,
  sunshine_h   REAL,
  daylight_h   REAL,
  overridden   INTEGER NOT NULL DEFAULT 0,
  lat          REAL NOT NULL DEFAULT 0,
  lon          REAL NOT NULL DEFAULT 0,
  place        TEXT NOT NULL DEFAULT '',
  fetched_at   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, date)
);
INSERT INTO weather_days_new (user_id, date, code, condition, t_max, t_min, precip_mm, sunshine_h, daylight_h, overridden, lat, lon, place, fetched_at)
  SELECT (SELECT id FROM _owner), date, code, condition, t_max, t_min, precip_mm, sunshine_h, daylight_h, overridden, lat, lon, place, fetched_at FROM weather_days;
DROP TABLE weather_days;
ALTER TABLE weather_days_new RENAME TO weather_days;

CREATE TABLE location_days_new (
  user_id      INTEGER REFERENCES users(id) ON DELETE CASCADE,
  date         TEXT NOT NULL,
  lat          REAL NOT NULL,
  lon          REAL NOT NULL,
  place        TEXT NOT NULL DEFAULT '',
  source       TEXT NOT NULL DEFAULT '',
  recorded_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, date)
);
INSERT INTO location_days_new (user_id, date, lat, lon, place, source, recorded_at)
  SELECT (SELECT id FROM _owner), date, lat, lon, place, source, recorded_at FROM location_days;
DROP TABLE location_days;
ALTER TABLE location_days_new RENAME TO location_days;

-- Personal settings (week start, journal prompts, reminder target, weather home location…)
-- move to a per-user table; the owner keeps what was configured. Only the app title and the
-- setup flag stay box-wide in `settings`. On a box with no account yet the keys stay where
-- they are and the first account to be created claims them.
CREATE TABLE IF NOT EXISTS user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);
INSERT OR IGNORE INTO user_settings (user_id, key, value)
  SELECT o.id, s.key, s.value FROM settings s, _owner o
  WHERE s.key IN ('week_start', 'landing', 'journal_prompt', 'bad_thing_enabled', 'bad_prompt',
                  'reminder_time', 'reminder_url', 'reminder_format', 'reminder_last', 'defaults_seeded',
                  'weather_enabled', 'weather_lat', 'weather_lon', 'weather_place', 'weather_units', 'weather_last');
DELETE FROM settings
  WHERE key IN ('week_start', 'landing', 'journal_prompt', 'bad_thing_enabled', 'bad_prompt',
                'reminder_time', 'reminder_url', 'reminder_format', 'reminder_last', 'defaults_seeded',
                'weather_enabled', 'weather_lat', 'weather_lon', 'weather_place', 'weather_units', 'weather_last')
    AND EXISTS (SELECT 1 FROM _owner);

-- Accounts made on first sight (a Pocket Core token, "Sign in with Pocket Core", a proxy
-- header) were all created as admins. Only the owner is; the others become members.
UPDATE users SET role = 'member'
  WHERE id <> (SELECT id FROM _owner) AND password_hash = '' AND role = 'admin';

DROP TABLE _owner;
