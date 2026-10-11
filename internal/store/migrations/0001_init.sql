CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'admin',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- A habit is a yes/no question asked every scheduled day: "did I run?".
-- start_date / end_date bound the months it appears on; swapping habits per
-- month (the notebook way) is just ending one and starting another.
CREATE TABLE habits (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  color      TEXT NOT NULL DEFAULT '',
  days       TEXT NOT NULL DEFAULT '1234567',   -- ISO weekdays the habit is scheduled on
  sort_order INTEGER NOT NULL DEFAULT 0,
  start_date TEXT NOT NULL,
  end_date   TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT
);

-- status: 1 = done (green), -1 = missed (red). No row = not marked yet.
CREATE TABLE habit_entries (
  habit_id   INTEGER NOT NULL REFERENCES habits(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  status     INTEGER NOT NULL CHECK (status IN (1, -1)),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (habit_id, date)
);
CREATE INDEX habit_entries_date ON habit_entries(date);

-- A tracker is a number recorded once per day: a 1-5 score (kind = scale)
-- or a free measurement such as weight (kind = number).
CREATE TABLE trackers (
  id               INTEGER PRIMARY KEY,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('scale', 'number')),
  min_value        REAL NOT NULL DEFAULT 1,
  max_value        REAL NOT NULL DEFAULT 5,
  step             REAL NOT NULL DEFAULT 1,
  unit             TEXT NOT NULL DEFAULT '',
  higher_is_better INTEGER NOT NULL DEFAULT 1,
  sort_order       INTEGER NOT NULL DEFAULT 0,
  start_date       TEXT NOT NULL,
  end_date         TEXT,
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at       TEXT
);

CREATE TABLE tracker_entries (
  tracker_id INTEGER NOT NULL REFERENCES trackers(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  value      REAL NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (tracker_id, date)
);
CREATE INDEX tracker_entries_date ON tracker_entries(date);

-- "What was great about today": one line per day.
CREATE TABLE journal (
  date       TEXT PRIMARY KEY,
  text       TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
