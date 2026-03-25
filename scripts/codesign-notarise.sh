#!/usr/bin/env bash
# codesign-notarise.sh — Codesign, notarise, and staple macOS artefacts.
#
# Usage:
#   scripts/codesign-notarise.sh sign     <app-path>
#   scripts/codesign-notarise.sh notarise <dmg-path>
#   scripts/codesign-notarise.sh verify   <app-path> [dmg-path]
#
# Subcommands:
#   sign      Codesign the .app with Developer ID (Hardened Runtime + timestamp).
#   notarise  Submit the DMG for notarisation, wait, then staple the ticket.
#   verify    Run codesign, spctl, and stapler validation checks.
#
# Environment / Make variables:
#   CODESIGN_IDENTITY   Developer ID Application identity (default: "Developer ID Application").
#   NOTARY_PROFILE      Keychain notarytool profile name  (default: "velocity-report").
#   NOTARY_KEY          App Store Connect API key path    (alternative to profile).
#   NOTARY_KEY_ID       API key ID                        (requires NOTARY_KEY).
#   NOTARY_ISSUER       API issuer ID                     (requires NOTARY_KEY).
#
# One-time local setup:
#   # Store notarisation credentials in the keychain:
#   xcrun notarytool store-credentials "velocity-report" \
#     --apple-id "<EMAIL>" --team-id "<TEAM_ID>" --password "<APP-SPECIFIC-PASSWORD>"
#
#   # Or use an App Store Connect API key:
#   export NOTARY_KEY=~/AuthKey_XXXX.p8
#   export NOTARY_KEY_ID=XXXXXXXXXX
#   export NOTARY_ISSUER=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────

CODESIGN_IDENTITY="${CODESIGN_IDENTITY:-Developer ID Application}"
NOTARY_PROFILE="${NOTARY_PROFILE:-velocity-report}"

# ── Helpers ───────────────────────────────────────────────────────────────────

step() { printf '\n── %s ──\n' "$*"; }
ok()   { printf '✓ %s\n' "$*"; }
die()  { printf 'Error: %s\n' "$*" >&2; exit 1; }

require_macos() {
  [ "$(uname)" = "Darwin" ] || die "This script requires macOS."
}

# Build the notarytool auth flags from environment.
# Prefers API-key auth if NOTARY_KEY is set; otherwise uses keychain profile.
notary_auth_flags() {
  if [ -n "${NOTARY_KEY:-}" ]; then
    [ -n "${NOTARY_KEY_ID:-}" ] || die "NOTARY_KEY is set but NOTARY_KEY_ID is missing."
    [ -n "${NOTARY_ISSUER:-}" ] || die "NOTARY_KEY is set but NOTARY_ISSUER is missing."
    echo "--key" "$NOTARY_KEY" "--key-id" "$NOTARY_KEY_ID" "--issuer" "$NOTARY_ISSUER"
  else
    # On macOS 13+, notarytool stores credentials in the Data Protection
    # keychain which is not searchable via `security find-generic-password`.
    # Skip the pre-flight check and let notarytool report its own error.
    echo "--keychain-profile" "$NOTARY_PROFILE"
  fi
}

# ── sign <app-path> ──────────────────────────────────────────────────────────

cmd_sign() {
  local app="${1:?usage: codesign-notarise.sh sign <app-path>}"
  [ -d "$app" ] || die "App bundle not found: $app"
  require_macos

  step "Codesigning $app"

  # Verify identity is available in the keychain.
  if ! security find-identity -v -p codesigning | grep -q "$CODESIGN_IDENTITY"; then
    die "Codesign identity not found: \"$CODESIGN_IDENTITY\".
Install a Developer ID Application certificate from the Apple Developer portal
and ensure it is present in the login or System keychain."
  fi

  # Sign nested frameworks/dylibs first (inside-out), then the top-level app.
  # --deep is unreliable for complex bundles; enumerate explicitly.
  local frameworks_dir="$app/Contents/Frameworks"
  if [ -d "$frameworks_dir" ]; then
    find "$frameworks_dir" \( -name '*.framework' -o -name '*.dylib' \) -print0 \
      | while IFS= read -r -d '' item; do
          codesign --force --options runtime --timestamp \
            --sign "$CODESIGN_IDENTITY" "$item"
        done
  fi

  codesign --force --options runtime --timestamp \
    --sign "$CODESIGN_IDENTITY" "$app"

  ok "Signed: $app"

  step "Verifying signature"
  codesign --verify --deep --strict --verbose=2 "$app"
  ok "codesign --verify passed"

  # spctl may fail if the cert isn't trusted by Gatekeeper yet (pre-notarisation).
  # Run it for information but don't fail the build.
  step "Gatekeeper assessment (pre-notarisation — may warn)"
  spctl --assess --type execute --verbose=4 "$app" 2>&1 || true
}

# ── notarise <dmg-path> ──────────────────────────────────────────────────────

cmd_notarise() {
  local dmg="${1:?usage: codesign-notarise.sh notarise <dmg-path>}"
  [ -f "$dmg" ] || die "DMG not found: $dmg"
  require_macos

  local -a auth
  # shellcheck disable=SC2207
  auth=($(notary_auth_flags))

  step "Submitting $dmg for notarisation"
  local output
  output=$(xcrun notarytool submit "$dmg" "${auth[@]}" --wait 2>&1) || true
  echo "$output"

  if echo "$output" | grep -q "status: Invalid"; then
    echo ""
    echo "Notarisation failed. Fetching log for details..."
    local sub_id
    sub_id=$(echo "$output" | grep "id:" | head -1 | awk '{print $2}')
    if [ -n "$sub_id" ]; then
      xcrun notarytool log "${auth[@]}" "$sub_id" 2>&1 || true
    fi
    die "Notarisation rejected by Apple. Check the log above."
  fi

  if ! echo "$output" | grep -q "status: Accepted"; then
    die "Unexpected notarisation status. Output:\n$output"
  fi

  ok "Notarisation accepted"

  step "Stapling notarisation ticket"
  xcrun stapler staple "$dmg"
  ok "Stapled: $dmg"

  step "Validating staple"
  xcrun stapler validate "$dmg"
  ok "Staple valid"
}

# ── verify <app-path> [dmg-path] ─────────────────────────────────────────────

cmd_verify() {
  local app="${1:?usage: codesign-notarise.sh verify <app-path> [dmg-path]}"
  local dmg="${2:-}"
  require_macos

  step "Verifying app signature: $app"
  codesign --verify --deep --strict --verbose=2 "$app"
  ok "codesign passed"

  step "Gatekeeper assessment: $app"
  spctl --assess --type execute --verbose=4 "$app"
  ok "spctl passed"

  if [ -n "$dmg" ]; then
    step "Gatekeeper assessment: $dmg"
    spctl --assess --type open --context context:primary-signature --verbose=4 "$dmg"
    ok "spctl (DMG) passed"

    step "Staple validation: $dmg"
    xcrun stapler validate "$dmg"
    ok "Staple valid"
  fi

  ok "All verification checks passed"
}

# ── Dispatch ─────────────────────────────────────────────────────────────────

subcmd="${1:-}"
shift || true

case "$subcmd" in
  sign)     cmd_sign "$@" ;;
  notarise) cmd_notarise "$@" ;;
  verify)   cmd_verify "$@" ;;
  *)
    echo "Usage: codesign-notarise.sh {sign|notarise|verify} <args...>" >&2
    exit 1
    ;;
esac
