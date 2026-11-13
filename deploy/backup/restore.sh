#!/bin/sh
# Obnova ze zálohy /backups/RRRR-MM-DD: přepíše databázi i datový svazek. Aplikace musí být zastavená a /data
# připojené pro zápis.
set -eu
src=/backups/${1:?použití: restore RRRR-MM-DD}
[ -f "$src/nanofaktura.dump" ] || { echo "restore: $src/nanofaktura.dump neexistuje" >&2; exit 1; }
echo "restore: databáze z $src"
dropdb --if-exists --force --maintenance-db=postgres "$PGDATABASE"
createdb --maintenance-db=postgres "$PGDATABASE"
pg_restore --no-owner --single-transaction -d "$PGDATABASE" "$src/nanofaktura.dump"
echo "restore: data z $src/data"
rsync -a --delete "$src/data/" /data/
chown -R 65532:65532 /data # uživatel nonroot v image aplikace
echo "restore: hotovo"
