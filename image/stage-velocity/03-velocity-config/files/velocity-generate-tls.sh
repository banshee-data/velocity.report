#!/bin/bash
# velocity-generate-tls.sh — Generate a local CA and server certificate
# for velocity.local. Runs via a systemd oneshot service before nginx
# starts on every boot.
#
# Creates:
#   $TLS_DIR/ca.key       — CA private key (root of trust)
#   $TLS_DIR/ca.crt       — CA certificate (install in browser to trust)
#   $TLS_DIR/server.key   — Server private key
#   $TLS_DIR/server.crt   — Server certificate (signed by CA)
#
# Idempotent: does nothing if server.crt already exists and is valid.
# On server cert renewal, the existing CA is reused so users do not
# need to re-trust the CA in their browser.

set -euo pipefail

TLS_DIR="${1:-/var/lib/velocity-report/tls}"
HOSTNAME="velocity.local"
CA_DAYS=3650    # 10 years
CERT_DAYS=825   # ~2.25 years (Apple max)

# Create TLS directory with restricted permissions from the start
(umask 077; mkdir -p "$TLS_DIR")

# Skip if server certificate already exists and has not expired,
# AND the CA key+cert are present and readable (preserves trust-once model)
if [ -f "$TLS_DIR/server.crt" ]; then
    if openssl x509 -in "$TLS_DIR/server.crt" -checkend 86400 -noout 2>/dev/null; then
        if [ -f "$TLS_DIR/ca.key" ] && [ -f "$TLS_DIR/ca.crt" ] \
            && [ -r "$TLS_DIR/ca.key" ] && [ -r "$TLS_DIR/ca.crt" ]; then
            exit 0
        fi
        echo "velocity-tls: CA key or cert missing — regenerating to preserve trust chain"
    else
        echo "velocity-tls: server certificate expired or expiring — regenerating"
    fi
fi

# --- CA: create only if missing or itself expiring ---
generate_ca=false
if [ ! -f "$TLS_DIR/ca.key" ] || [ ! -f "$TLS_DIR/ca.crt" ]; then
    generate_ca=true
elif ! openssl x509 -in "$TLS_DIR/ca.crt" -checkend 86400 -noout 2>/dev/null; then
    echo "velocity-tls: CA certificate expired or expiring — regenerating CA"
    generate_ca=true
fi

if [ "$generate_ca" = true ]; then
    echo "velocity-tls: generating CA for $HOSTNAME"
    (umask 077; openssl ecparam -genkey -name prime256v1 -out "$TLS_DIR/ca.key" 2>/dev/null)
    openssl req -new -x509 -key "$TLS_DIR/ca.key" \
        -out "$TLS_DIR/ca.crt" \
        -days "$CA_DAYS" \
        -subj "/CN=velocity.report Local CA" \
        -sha256 \
        -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
        -addext "keyUsage=critical,keyCertSign,cRLSign" \
        2>/dev/null
fi

echo "velocity-tls: generating server certificate for $HOSTNAME"

# Generate server key with restrictive umask
(umask 077; openssl ecparam -genkey -name prime256v1 -out "$TLS_DIR/server.key" 2>/dev/null)

# Generate server CSR and sign with CA
openssl req -new -key "$TLS_DIR/server.key" \
    -out "$TLS_DIR/server.csr" \
    -subj "/CN=$HOSTNAME" \
    -sha256 2>/dev/null

openssl x509 -req -in "$TLS_DIR/server.csr" \
    -CA "$TLS_DIR/ca.crt" \
    -CAkey "$TLS_DIR/ca.key" \
    -CAcreateserial \
    -out "$TLS_DIR/server.crt" \
    -days "$CERT_DAYS" \
    -sha256 \
    -extfile <(printf "subjectAltName=DNS:%s,DNS:localhost,IP:127.0.0.1\nbasicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth" "$HOSTNAME") \
    2>/dev/null

# Clean up CSR and serial (not needed at runtime)
rm -f "$TLS_DIR/server.csr" "$TLS_DIR/ca.srl"

# Restrict permissions — only the service user needs to read these
chmod 600 "$TLS_DIR/ca.key" "$TLS_DIR/server.key"
chmod 644 "$TLS_DIR/ca.crt" "$TLS_DIR/server.crt"

echo "velocity-tls: certificates ready in $TLS_DIR"
echo "velocity-tls: trust the CA by downloading https://$HOSTNAME/ca.crt"
