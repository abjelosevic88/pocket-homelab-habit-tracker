-- Optional second journal line: one thing that was bad today (off by default, Settings → General).
ALTER TABLE journal ADD COLUMN bad_text TEXT NOT NULL DEFAULT '';
