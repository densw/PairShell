#!/bin/sh
set -eu
test "$(id -u)" -eq 0
if [ "$#" -ne 2 ]; then
  printf '%s\n' 'Usage: install-hosthatch-turn.sh RELAY_FQDN RELAY_PUBLIC_IP' >&2
  exit 2
fi
version=$(dpkg-query -W -f='${Version}' coturn)
if ! dpkg --compare-versions "${version#*:}" ge 4.19.2; then
  printf '%s\n' 'Install coturn 4.19.2 or later from a trusted source, or use deploy/compose.yaml.' >&2
  exit 1
fi
umask 077
export PAIRSHELL_INSTALL_RELAY_HOST="$1" PAIRSHELL_INSTALL_RELAY_IP="$2"
python3 - <<'PY'
from ipaddress import ip_address
from os import environ, replace
from pathlib import Path
from re import fullmatch
from tempfile import NamedTemporaryFile

config = Path('/etc/pairshell')
host = environ['PAIRSHELL_INSTALL_RELAY_HOST']
address = ip_address(environ['PAIRSHELL_INSTALL_RELAY_IP'])
if not fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]{0,252}', host) or address.version != 4:
    raise SystemExit('Provide a relay hostname and an IPv4 address')
env = config.joinpath('environment').read_text()
secret = next(line.split('=', 1)[1] for line in env.splitlines()
              if line.startswith('PAIRSHELL_TURN_SECRET='))
if not fullmatch(r'[A-Za-z0-9_-]{32,}', secret):
    raise SystemExit('Expected an unquoted, high-entropy URL-safe TURN secret')
template = Path('/root/pairshell-stage/turnserver.conf.template').read_text()
sample = (template.replace('realm=relay.example', 'realm='+host)
          .replace('external-ip=203.0.113.10', 'external-ip='+str(address))
          .replace('REPLACE_WITH_PAIRSHELL_TURN_SECRET', secret)
          .replace('/certs/', '/etc/pairshell/'))
with NamedTemporaryFile(dir=config, delete=False, mode='w') as temp:
    temp.write(sample+'\nno-stdout-log\nlog-file=syslog\n')
replace(temp.name, config/'turnserver.conf')
PY
chown root:pairshell /etc/pairshell/turnserver.conf
chmod 640 /etc/pairshell/turnserver.conf
systemctl disable --now coturn
cat > /etc/systemd/system/pairshell-turn.service <<'UNIT'
[Unit]
Description=PairShell TURN relay
After=network-online.target
[Service]
User=turnserver
SupplementaryGroups=pairshell
ExecStart=/usr/bin/turnserver -c /etc/pairshell/turnserver.conf --pidfile=
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now pairshell-turn
systemctl restart pairshelld
systemctl is-active pairshell-turn pairshelld
