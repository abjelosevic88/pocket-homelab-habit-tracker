# Backup & restore

Everything lives in `DATA_DIR` (`/data` in the container):

```
/data/pocket-habits.db        # the SQLite database (WAL mode: -wal / -shm files appear while running)
/data/backups/                # VACUUM INTO copies made by the CLI
```

## Making a backup

- **UI:** Settings → Backup & data → *Download database*.
- **CLI:** `docker exec pocket-habits pocket-habits backup` (or `backup -o /data/backups/named.db`).
- **API:** `GET /api/v1/backup` with a token.
- **Portable:** `GET /api/v1/export.json` — plain JSON you can re-import on another instance.

All of these produce a consistent snapshot even while the app is writing. Copying `pocket-habits.db` raw while the container runs can give you a torn file because of the WAL.

### With restic / Borg / Backrest / Kopia

Add `DATA_DIR` to your backup set, **exclude** `pocket-habits.db*`, and run the CLI backup (or the sqlite3 `.backup` command) as a pre-snapshot hook so the consistent copy under `backups/` is what gets picked up. Prune `backups/` as you like — each copy is a full database (tens of KB to a few MB).

## Restoring

```bash
docker compose stop app
cp pocket-habits-20261011-030000.db data/pocket-habits.db
rm -f data/pocket-habits.db-wal data/pocket-habits.db-shm
docker compose start app
```

Or start a fresh instance and use Settings → Backup & data → *Import JSON*.
