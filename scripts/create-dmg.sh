#!/usr/bin/env bash
# create-dmg.sh — Package a .app bundle into a drag-to-install DMG.
#
# Usage:
#   scripts/create-dmg.sh <app-path> <dmg-path> <volume-name> [extras...]
#
# Any additional paths after the volume name are copied into the DMG root.
#
# The resulting DMG opens in a small Finder window with the app icon on the
# left, any extras in the centre, and an Applications shortcut on the right,
# ready for drag-and-drop installation. No background image is used.
set -euo pipefail

APP_PATH="${1:?usage: create-dmg.sh <app-path> <dmg-path> <volume-name> [extras...]}"
DMG_PATH="${2:?usage: create-dmg.sh <app-path> <dmg-path> <volume-name> [extras...]}"
VOLUME_NAME="${3:?usage: create-dmg.sh <app-path> <dmg-path> <volume-name> [extras...]}"
shift 3
EXTRAS=("${@+"$@"}")

if [ "$(uname)" != "Darwin" ]; then
  echo "Error: create-dmg.sh requires macOS" >&2
  exit 1
fi

if [ ! -d "$APP_PATH" ]; then
  echo "Error: app bundle not found: $APP_PATH" >&2
  exit 1
fi

APP_NAME="$(basename "$APP_PATH")"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Detach any stale volume with the same name from a previous run.
mount_point="/Volumes/$VOLUME_NAME"
if [ -d "$mount_point" ]; then
  echo "Detaching stale volume: $mount_point" >&2
  hdiutil detach "$mount_point" -force 2>/dev/null || true
fi

# Remove any existing DMG so hdiutil convert does not prompt.
rm -f "$DMG_PATH"

# ── 1. Stage contents ────────────────────────────────────────────────────────
staging=$(mktemp -d)
raw_dmg=""
device=""
cleanup() {
  [ -n "$device" ]    && hdiutil detach "$device" -force 2>/dev/null || true
  [ -n "$raw_dmg" ]   && rm -f "$raw_dmg"
  [ -n "$staging" ]   && rm -rf "$staging"
}
trap cleanup EXIT

cp -R "$APP_PATH" "$staging/$APP_NAME"
ln -s /Applications "$staging/Applications"

# Copy any extra files (e.g. Getting Started guide) into the DMG root.
extra_names=()
if [ ${#EXTRAS[@]} -gt 0 ]; then
  for extra in "${EXTRAS[@]}"; do
    if [ -e "$extra" ]; then
      cp -R "$extra" "$staging/"
      extra_names+=("$(basename "$extra")")
    else
      echo "Warning: extra file not found, skipping: $extra" >&2
    fi
  done
fi

# ── 2. Create writable DMG ───────────────────────────────────────────────────
# Size the image to fit the app bundle plus headroom.
# Atomically reserve a unique path, then rename to .dmg for hdiutil.
raw_base=$(mktemp /tmp/dmg-rw-XXXXXX)
raw_dmg="${raw_base}.dmg"
mv "$raw_base" "$raw_dmg"
app_size_kb=$(du -sk "$staging/$APP_NAME" | awk '{print $1}')
dmg_size_kb=$(( app_size_kb + 8192 ))  # 8 MiB headroom

hdiutil create \
  -volname "$VOLUME_NAME" \
  -srcfolder "$staging" \
  -fs HFS+ \
  -format UDRW \
  -size "${dmg_size_kb}k" \
  -ov \
  "$raw_dmg"

# ── 3. Mount and configure Finder layout ─────────────────────────────────────
device=$(hdiutil attach -readwrite -noverify -noautoopen "$raw_dmg" \
  | grep '/Volumes/' | head -1 | awk '{print $1}')

# Wait briefly for the volume to become available.
for _ in 1 2 3 4 5; do
  [ -d "$mount_point" ] && break
  sleep 1
done

if [ ! -d "$mount_point" ]; then
  echo "Error: volume did not mount at $mount_point" >&2
  exit 1
fi

# Remove macOS filesystem metadata that should not appear in the DMG.
rm -rf "$mount_point/.fseventsd"

# Give Finder time to index the newly mounted volume.
sleep 2

# Apply Finder view settings via AppleScript.
# Window: 520 × 400, icon view, 72 px icons, no toolbar/sidebar.
# Row 1 (y=130): app icon at left, Applications alias at right.
# Row 2 (y=260): extras centred below.
extra_args=()
for name in "${extra_names[@]+"${extra_names[@]}"}"; do
  extra_args+=("$name")
done

osascript "$SCRIPT_DIR/dmg-layout.applescript" \
  "$VOLUME_NAME" "$APP_NAME" ${extra_args[@]+"${extra_args[@]}"}

# Remove macOS metadata that Finder/fsevents recreates during layout.
rm -rf "$mount_point/.fseventsd" "$mount_point/.Trashes"
# Place a sentinel file to prevent fseventsd from recreating the directory.
touch "$mount_point/.fseventsd"

# Ensure .DS_Store is flushed.
sync

hdiutil detach "$device"
device=""

# ── 4. Convert to compressed read-only DMG ───────────────────────────────────
mkdir -p "$(dirname "$DMG_PATH")"
hdiutil convert "$raw_dmg" -format UDZO -o "$DMG_PATH" -ov
rm -f "$raw_dmg"
raw_dmg=""

echo "✓ DMG created: $DMG_PATH"
