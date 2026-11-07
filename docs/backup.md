# Backup

Everything lives in `DATA_DIR`: the SQLite file `pocket-trails.db` (WAL mode) and the photos and PDFs in `files/<person>/<id>`. Take a consistent copy of the database with `pocket-trails backup` (writes `DATA_DIR/backups/pocket-trails-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin), and back up `files/` as it is. Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-trails.db` (remove `-wal`/`-shm` files) and `files/`, start.

Every person can also export their own trails as JSON (`/api/v1/export.json`, back in with Settings → Backup & data, twice is the same as once), and their outings as CSV (`/api/v1/export.csv`). A route downloads as GPX on its page.
