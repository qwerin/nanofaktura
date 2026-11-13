# Zálohování serveru

Zálohuje se databáze (Postgres) a datový svazek aplikace (`appdata`, v kontejneru `/data`: přílohy a `secret.key`) —
to je veškerý stav instance. Je to záloha **celé instance** pro případ havárie; přenos jednoho účtu mezi instancemi řeší
*Nastavení → Záloha a přenos* (`nanofaktura backup export|import`, SPEC §7.16).

Zálohy obsahují účetní data, osobní údaje a šifrovací klíč `secret.key` (jím jsou v databázi zašifrované tokeny Fio a
certifikáty pro EET). Adresáře mají práva 700; nekopírujte je jinam než na server a do PC správce.
Pokud klíč předáváte proměnnou `NANOFAKTURA_SECRET_KEY`, v záloze není — uschovejte ho zvlášť, bez něj tajemství
z obnovené databáze nepůjdou dešifrovat.

## Na serveru

Služba `backup` v `docker-compose.yml` (image se sestaví z `deploy/backup/` spolu s aplikací) jednou denně
(`BACKUP_SCHEDULE`, výchozí 2:30 Europe/Prague) uloží do `BACKUP_DIR` na hostiteli (výchozí `/srv/nanofaktura-backups`):

```
/srv/nanofaktura-backups/
  2026-11-13/nanofaktura.dump   pg_dump -Fc celé databáze
  2026-11-13/data/              kopie datového svazku; nezměněné soubory jsou hardlinky na předchozí den
  last-ok                       čas poslední úspěšné zálohy
```

- Ponechá se posledních `BACKUP_KEEP_DAYS` (14) dní a první záloha z každého z posledních `BACKUP_KEEP_MONTHS` (12)
  měsíců. Díky hardlinkům zabírá každý den jen databázi a nové přílohy.
- Při startu kontejneru se záloha udělá hned, pokud poslední je starší než 24 hodin.
- Healthcheck: kontejner je `unhealthy`, když poslední úspěšná záloha je starší než 26 hodin (vidět v Portaineru).
- Ruční záloha: `docker exec nanofaktura-backup-1 backup`. Výpis: `docker logs nanofaktura-backup-1`.
- `pg_dump` musí mít stejnou hlavní verzi jako databáze: při změně obrazu `db` v `docker-compose.yml` změňte i
  `deploy/backup/Dockerfile`.

## Stažení do PC

Záloha jen na serveru nepřežije ztrátu serveru, proto se stahuje do PC. `scripts/backup-pull.sh` přes SSH a rsync
stáhne nové dny (soubory zase jako hardlinky, takže se přenáší jen to nové):

```bash
./scripts/backup-pull.sh <server>                       # do ~/zalohy/nanofaktura
./scripts/backup-pull.sh <server> /mnt/disk/nanofaktura # jinam
```

- `<server>` je host z `~/.ssh/config`; uživatel musí číst `BACKUP_DIR` (vlastní ho root, práva 700).
- Co server už smazal, v PC zůstane: v PC se drží posledních `KEEP_DAYS` (30) dní a první záloha každého měsíce navždy.
- Skript skončí chybou (a vypíše POZOR), když poslední záloha na serveru je starší než 2 dny — tak se pozná, že
  zálohování na serveru neběží.

Automaticky jednou denně (systemd uživatele; `Persistent=true` dožene běh, když byl PC vypnutý):

```ini
# ~/.config/systemd/user/nanofaktura-backup.service
[Unit]
Description=Stažení záloh NanoFaktury
[Service]
Type=oneshot
ExecStart=/cesta/k/repu/scripts/backup-pull.sh <server>

# ~/.config/systemd/user/nanofaktura-backup.timer
[Unit]
Description=Denní stažení záloh NanoFaktury
[Timer]
OnCalendar=*-*-* 09:00
Persistent=true
[Install]
WantedBy=timers.target
```

```bash
systemctl --user daemon-reload && systemctl --user enable --now nanofaktura-backup.timer
journalctl --user -u nanofaktura-backup   # výsledek posledních běhů
```

Služba neběží v sezení s ssh-agentem: použijte klíč bez hesla určený jen pro zálohy, nebo v service nastavte
`Environment=SSH_AUTH_SOCK=%t/ssh-agent.socket` (podle toho, kde agent běží).

## Obnova

Přepíše databázi i datový svazek stavem ze zvoleného dne. Aplikace musí stát (v Portaineru zastavit kontejner `app`,
nebo `docker stop nanofaktura-app-1`). Názvy sítě, svazku a image zálohy odpovídají stacku pojmenovanému `nanofaktura`
(ověření: `docker inspect nanofaktura-backup-1`):

```bash
docker stop nanofaktura-app-1
docker run --rm --network nanofaktura_internal -v nanofaktura_appdata:/data -v /srv/nanofaktura-backups:/backups \
  -e PGHOST=db -e PGUSER=nanofaktura -e PGDATABASE=nanofaktura -e PGPASSWORD=<heslo> \
  --entrypoint restore nanofaktura-backup 2026-11-13
docker start nanofaktura-app-1
```

Bez Traefik overlay se síť jmenuje `nanofaktura_default`. Heslo DB lze převzít z kontejneru zálohy:
`docker exec nanofaktura-backup-1 printenv PGPASSWORD`. Obnova databázi smaže a založí znovu, takže po ní nic
nezůstane z novějšího stavu. Ze zálohy v PC ji obnovíte tak, že adresář dne nahrajete na server
(`rsync -aH ~/zalohy/nanofaktura/2026-11-13 <server>:/srv/nanofaktura-backups/`).

Obsah dumpu bez obnovy: `pg_restore -l nanofaktura.dump`; do jiné databáze (např. pro kontrolu) `pg_restore --no-owner -d …`.
