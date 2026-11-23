#!/bin/sh
set -eu
test "$(id -u)" -eq 0
if [ "$#" -ne 3 ]; then
  printf '%s\n' 'Usage: install-hosthatch.sh BINARY_SHA256 TLS_SOURCE_DIR TURN_URLS' >&2
  exit 2
fi
case "$2" in /*) ;; *) exit 2 ;; esac
bin=/root/pairshell-stage/pairshelld
test -f "$bin"
test ! -L "$bin"
test -r "$2/fullchain.pem"
test -r "$2/privkey.pem"
printf '%s  %s\n' "$1" "$bin" | sha256sum --check --strict --status
export PAIRSHELL_INSTALL_TURN_URLS="$3"
umask 077
id pairshell >/dev/null 2>&1 || useradd --system --home /var/lib/pairshell --shell /usr/sbin/nologin pairshell
install -d -m 750 -o root -g pairshell /etc/pairshell
install -d -m 700 -o pairshell -g pairshell /var/lib/pairshell
install -d -m 755 /opt/pairshell
install -m 755 "$bin" /opt/pairshell/pairshelld
printf '%s\n' "$2" > /etc/pairshell/certificate-source
python3 - <<'PY'
import os,secrets
p='/etc/pairshell/environment'
if not os.path.exists(p):
 with open(p,'x') as f:
  for k in ['CREATE_TOKEN','WITNESS_TOKEN','TURN_SECRET']:
   f.write('PAIRSHELL_'+k+'='+secrets.token_urlsafe(32)+'\n')
  f.write('PAIRSHELL_TURN_URLS='+os.environ['PAIRSHELL_INSTALL_TURN_URLS']+'\n')
 os.chmod(p,0o600)
PY
cat > /opt/pairshell/sync-certificate.sh <<'SH'
#!/bin/sh
set -eu
IFS= read -r source < /etc/pairshell/certificate-source
install -m 640 -o root -g pairshell "$source/fullchain.pem" /etc/pairshell/fullchain.pem
install -m 640 -o root -g pairshell "$source/privkey.pem" /etc/pairshell/privkey.pem
SH
chmod 700 /opt/pairshell/sync-certificate.sh
/opt/pairshell/sync-certificate.sh
cat > /etc/systemd/system/pairshelld.service <<'UNIT'
[Unit]
Description=PairShell coordination and witness
After=network-online.target
[Service]
User=pairshell
Group=pairshell
EnvironmentFile=/etc/pairshell/environment
WorkingDirectory=/var/lib/pairshell
ExecStart=/opt/pairshell/pairshelld --listen 0.0.0.0:9443 --tls-cert /etc/pairshell/fullchain.pem --tls-key /etc/pairshell/privkey.pem --witness-dir /var/lib/pairshell/receipts --witness-key /var/lib/pairshell/witness.key
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/pairshell
[Install]
WantedBy=multi-user.target
UNIT
install -d /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/pairshell.sh <<'SH'
#!/bin/sh
set -eu
/opt/pairshell/sync-certificate.sh
systemctl try-restart pairshelld.service
systemctl try-restart pairshell-turn.service
SH
chmod 700 /etc/letsencrypt/renewal-hooks/deploy/pairshell.sh
systemctl daemon-reload
systemctl enable --now pairshelld.service
systemctl is-active pairshelld.service
