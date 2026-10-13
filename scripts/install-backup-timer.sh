#!/usr/bin/env bash
set -euo pipefail

# Install the daily backup systemd timer. Run as root on the VPS.
#   STASH_DIR=/opt/stash ./scripts/install-backup-timer.sh
STASH_DIR="${STASH_DIR:-/opt/stash}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "This script must run as root (it installs systemd units)." >&2
  exit 1
fi

for unit in stash-backup.service stash-backup.timer; do
  src="$STASH_DIR/deploy/$unit"
  test -f "$src" || { echo "missing $src" >&2; exit 1; }
  install -m 0644 "$src" "/etc/systemd/system/$unit"
done

systemctl daemon-reload
systemctl enable --now stash-backup.timer

echo "Installed. Next run:"
systemctl list-timers stash-backup.timer --no-pager
echo
echo "Run one now with: systemctl start stash-backup.service"
