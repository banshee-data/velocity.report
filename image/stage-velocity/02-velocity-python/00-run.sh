#!/bin/bash -e
# 02-velocity-python/00-run.sh — Create report output directory
#
# The Go PDF pipeline writes generated reports to this directory.
# Owned by the velocity service user so the Go server can write here.

on_chroot << 'CHEOF'
if ! id velocity >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/velocity-report --shell /usr/sbin/nologin velocity
fi

mkdir -p /var/lib/velocity-report/reports/output
chown -R velocity:velocity /var/lib/velocity-report/reports
CHEOF
