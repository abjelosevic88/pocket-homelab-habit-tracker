-- A value assumed for every day the tracker is active but has no entry
-- (e.g. "Stomach problem?" defaults to no). NULL = no default.
ALTER TABLE trackers ADD COLUMN default_value REAL;
