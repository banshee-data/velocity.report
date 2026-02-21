#!/usr/bin/env bash
# create-dmg.sh — Package a .app bundle into a drag-to-install DMG.
#
# Usage:
#   scripts/create-dmg.sh <app-path> <dmg-path> <volume-name>
#
# The resulting DMG opens in a small Finder window with the app icon on the
# left and an Applications shortcut on the right, ready for drag-and-drop
# installation. No background image is used.
set -euo pipefail

APP_PATH="${1:?usage: create-dmg.sh <app-path> <dmg-path> <volume-name>}"
DMG_PATH="${2:?usage: create-dmg.sh <app-path> <dmg-path> <volume-name>}"
VOLUME_NAME="${3:?usage: create-dmg.sh <app-path> <dmg-path> <volume-name>}"

if [ "$(uname)" != "Darwin" ]; then
  echo "Error: create-dmg.sh requires macOS" >&2
  exit 1
fi

if [ ! -d "$APP_PATH" ]; then
  echo "Error: app bundle not found: $APP_PATH" >&2
  exit 1
fi

APP_NAME="$(basename "$APP_PATH")"

# ── 1. Stage contents ────────────────────────────────────────────────────────
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT

cp -R "$APP_PATH" "$staging/$APP_NAME"
ln -s /Applications "$staging/Applications"

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
mount_point="/Volumes/$VOLUME_NAME"

# Wait briefly for the volume to become available.
for _ in 1 2 3 4 5; do
  [ -d "$mount_point" ] && break
  sleep 1
done

if [ ! -d "$mount_point" ]; then
  echo "Error: volume did not mount at $mount_point" >&2
  hdiutil detach "$device" -force 2>/dev/null || true
  rm -f "$raw_dmg"
  exit 1
fi

# Apply Finder view settings via AppleScript.
# Window: 480 × 320, icon view, 80 px icons, no toolbar/sidebar.
# App icon at (120, 160), Applications at (360, 160) — centred side-by-side.
osascript <<EOF
tell application "Finder"
  tell disk "$VOLUME_NAME"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set bounds of container window to {100, 100, 580, 420}
    set theViewOptions to icon view options of container window
    set arrangement of theViewOptions to not arranged
    set icon size of theViewOptions to 80
    set position of item "$APP_NAME" of container window to {120, 160}
    set position of item "Applications" of container window to {360, 160}
    close
    open
    update without registering applications
    delay 2
    close
  end tell
end tell
EOF

# Ensure .DS_Store is flushed.
sync

hdiutil detach "$device"

# ── 4. Convert to compressed read-only DMG ───────────────────────────────────
mkdir -p "$(dirname "$DMG_PATH")"
hdiutil convert "$raw_dmg" -format UDZO -o "$DMG_PATH" -ov
rm -f "$raw_dmg"

echo "✓ DMG created: $DMG_PATH"
