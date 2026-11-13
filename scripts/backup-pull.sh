#!/bin/sh
# Stáhne zálohy ze serveru do PC (docs/BACKUP.md): ./scripts/backup-pull.sh <ssh-server> [cílový adresář]
# Stahuje jen nové dny (rsync, soubory jako hardlinky). Zálohy smazané na serveru v PC zůstanou; v PC se ponechá
# posledních KEEP_DAYS dní a první záloha z každého měsíce bez omezení.
set -eu
server=${1:?použití: backup-pull.sh <ssh-server> [cílový adresář]}
dest=${2:-$HOME/zalohy/nanofaktura}
src=${BACKUP_DIR:-/srv/nanofaktura-backups}
KEEP_DAYS=${KEEP_DAYS:-30}

mkdir -p "$dest"
chmod 700 "$dest" # účetní data, osobní údaje a šifrovací klíč
rsync -aH --exclude='.tmp-*' "$server:$src/" "$dest/"

all=$(ls -1d "$dest"/20??-??-?? 2>/dev/null | sort)
keep=$({
	echo "$all" | tail -n "$KEEP_DAYS"
	echo "$all" | awk '{ m = substr($0, length($0) - 9, 7) } !(m in s) { s[m] = 1; print }'
})
for d in $all; do
	echo "$keep" | grep -qxF "$d" || rm -rf "$d"
done

last=$(cat "$dest/last-ok" 2>/dev/null || echo "nikdy")
echo "zálohy v $dest: $(ls -1d "$dest"/20??-??-?? 2>/dev/null | wc -l) dní, $(du -sh "$dest" | cut -f1), poslední úspěšná na serveru: $last"
if ! find "$dest/last-ok" -mmin -2880 2>/dev/null | grep -q .; then
	echo "POZOR: poslední záloha na serveru je starší než 2 dny" >&2
	exit 1
fi
