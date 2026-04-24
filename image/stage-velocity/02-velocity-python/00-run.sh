#!/bin/bash -e
# 02-velocity-report-dirs/00-run.sh — Create report output directory
#
# The Go PDF pipeline writes generated reports to this directory.
# Owned by the velocity service user so the Go server can write here.

on_chroot << 'CHEOF'
if ! id velocity >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/velocity-report --shell /usr/sbin/nologin velocity
fi

mkdir -p /opt/velocity-report/tools/pdf-generator/output
chown velocity:velocity /opt/velocity-report/tools/pdf-generator/output
CHEOF
