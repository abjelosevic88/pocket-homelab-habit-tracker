-- pragma: no-transaction
-- Add the yes/no tracker kind. SQLite cannot alter a CHECK constraint, so the
-- table is rebuilt; foreign keys are off during this migration so the cascade
-- on tracker_entries does not fire.
CREATE TABLE trackers_new (
  id               INTEGER PRIMARY KEY,
  name             TEXT NOT NULL,
  kind             TEXT NOT NULL CHECK (kind IN ('scale', 'number', 'yesno')),
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
INSERT INTO trackers_new SELECT id, name, kind, min_value, max_value, step, unit, higher_is_better, sort_order, start_date, end_date, created_at, updated_at FROM trackers;
DROP TABLE trackers;
ALTER TABLE trackers_new RENAME TO trackers;
