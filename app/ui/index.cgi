#!/bin/bash
APP_DEST="${TRIM_APPDEST:-/var/apps/wg-server/target}"
PKG_VAR="${TRIM_PKGVAR:-/var/apps/wg-server/var}"
export UI_DIR="${APP_DEST}/ui"
export TRIM_PKGVAR="${PKG_VAR}"
exec "${APP_DEST}/backend/wg-server"
