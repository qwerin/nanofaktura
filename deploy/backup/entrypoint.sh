#!/bin/sh
# Spustí zálohu podle BACKUP_SCHEDULE (cron, časové pásmo TZ). Chybí-li záloha za posledních 24 h, udělá ji hned.
set -eu
echo "$BACKUP_SCHEDULE /usr/local/bin/backup >/proc/1/fd/1 2>&1" > /etc/crontabs/root
if ! find /backups/last-ok -mmin -1440 2>/dev/null | grep -q .; then
	/usr/local/bin/backup || echo "backup: úvodní záloha selhala" >&2
fi
exec crond -f -l 8
