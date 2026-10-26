#!/bin/sh
set -e
mkdir -p /data
chown -R mantenimiento:mantenimiento /data
exec su-exec mantenimiento "$@"
