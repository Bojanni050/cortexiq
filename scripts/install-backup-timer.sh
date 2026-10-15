#!/usr/bin/env bash
set -euo pipefail

# Install the daily backup systemd timer. Run as root on the VPS.
#   CORTEXIQ_DIR=/opt/cortexiq ./scripts/install-backup-timer.sh
# When CORTEXIQ_DIR is unset the deployment is detected: /opt/stash (the layout
# created before the rebrand) when it exists, otherwise /opt/cortexiq.
if [[ "$(id -u)" -ne 0 ]]; then
  echo "This script must run as root (it installs systemd units)." >&2
  exit 1
fi

if [[ -z "${CORTEXIQ_DIR:-}" ]]; then
  if [[ -d /opt/stash ]]; then CORTEXIQ_DIR=/opt/stash; else CORTEXIQ_DIR=/opt/cortexiq; fi
fi

for unit in cortexiq-backup.service cortexiq-backup.timer; do
  src="$CORTEXIQ_DIR/deploy/$unit"
  test -f "$src" || { echo "missing $src" >&2; exit 1; }
  # The units ship with /opt/cortexiq; point them at the deployment that exists.
  sed "s|/opt/cortexiq|$CORTEXIQ_DIR|g" "$src" > "/etc/systemd/system/$unit"
  chmod 0644 "/etc/systemd/system/$unit"
done

# Retire the pre-rebrand units so an old and a new timer cannot both fire.
for old in stash-backup.timer stash-backup.service; do
  if [[ -f "/etc/systemd/system/$old" ]]; then
    systemctl disable --now "$old" 2>/dev/null || true
    rm -f "/etc/systemd/system/$old"
  fi
done

systemctl daemon-reload
systemctl enable --now cortexiq-backup.timer

echo "Installed from $CORTEXIQ_DIR. Next run:"
systemctl list-timers cortexiq-backup.timer --no-pager
echo
echo "Run one now with: systemctl start cortexiq-backup.service"
