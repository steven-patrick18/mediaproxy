#!/bin/bash
# /usr/local/sbin/mediaproxy-pgbackup-offsite (example)
# Copy this and edit. Mediaproxy-pgbackup runs daily and writes a gzip to
# /var/backups/mediaproxy/ — this script picks up the newest one and ships
# it off-site via rclone. Configure rclone first with `rclone config`
# (S3, B2, GDrive — anything rclone supports works).
set -euo pipefail

REMOTE="${MEDIAPROXY_BACKUP_REMOTE:-b2:mediaproxy-backups}"  # rclone remote
DIR=/var/backups/mediaproxy

NEWEST=$(ls -1t "$DIR"/mediaproxy-*.sql.gz 2>/dev/null | head -1 || true)
if [ -z "$NEWEST" ]; then
    echo "no local backup found in $DIR" >&2
    exit 1
fi

rclone copyto --quiet "$NEWEST" "$REMOTE/$(basename "$NEWEST")"

# Retention: keep last 30 days on the remote.
rclone delete --min-age 30d "$REMOTE" 2>/dev/null || true
