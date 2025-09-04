#!/bin/sh
set -e

# Persist a stable ID in /var/lib/machine-id (backed by a named volume)
mkdir -p /var/lib/machine-id
if [ ! -s /var/lib/machine-id/machine-id ]; then
  cat /proc/sys/kernel/random/uuid | tr -d '-' | tr '[:upper:]' '[:lower:]' > /var/lib/machine-id/machine-id
fi

# Ensure /etc/machine-id points to it
if [ ! -e /etc/machine-id ]; then
  ln -s /var/lib/machine-id/machine-id /etc/machine-id
fi

exec /app/client "$@"
