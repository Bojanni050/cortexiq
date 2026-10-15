#!/usr/bin/env bash
set -euo pipefail

# Install the daily backup systemd timer. Run as root on the VPS.
#   CORTEXIQ_DIR=/opt/cortexiq ./scripts/install-backup-timer.sh
# When CORTEXIQ_DIR is unset it defaults to /opt/cortexiq.
if [[ "$(id -u)" -ne 0 ]]; then
  echo "This script must run as root (it installs systemd units)." >&2
  exit 1
fi

CORTEXIQ_DIR="${CORTEXIQ_DIR:-/opt/cortexiq}"

for unit in cortexiq-backup.service cortexiq-backup.timer; do
  src="$CORTEXIQ_DIR/deploy/$unit"
  test -f "$src" || { echo "missing $src" >&2; exit 1; }
  # The units ship with /opt/cortexiq; point them at the deployment that exists.
  sed "s|/opt/cortexiq|$CORTEXIQ_DIR|g" "$src" > "/etc/systemd/system/$unit"
  chmod 0644 "/etc/systemd/system/$unit"
done

systemctl daemon-reload
systemctl enable --now cortexiq-backup.timer

echo "Installed from $CORTEXIQ_DIR. Next run:"
systemctl list-timers cortexiq-backup.timer --no-pager
echo
echo "Run one now with: systemctl start cortexiq-backup.service"
