#!/usr/bin/env bash
# Install remotr-agent on a Linux endpoint and enroll with the Remotr server.
#
#   REMOTR_YES=1 REMOTR_SERVER_URL=https://remotr.example:8443 \
#   REMOTR_DEPLOYMENT_TOKEN='uuid.hexsecret' \
#   bash <(curl -fsSL https://raw.githubusercontent.com/DavidHoenisch/remotr/master/scripts/install-agent.sh)
#
# CA is downloaded from ${REMOTR_SERVER_URL}/v1/ca.pem by default (public cert, not secret).
# Optional: REMOTR_CA_FINGERPRINT=ab:cd:... to pin the CA after first fetch.
# Override: REMOTR_CA_FILE, REMOTR_CA_PEM, or REMOTR_CA_URL
#
# From a clone:
#   sudo ./scripts/install-agent.sh
#
# Documentation: docs/guides/installing-agent.md
#
# Requires: root, systemd, curl, tar, install(1)
# Optional: jq (for REMOTR_VERSION=latest), sha256sum (checksum verify), openssl (CA pin)
#
# Environment:
#   REMOTR_SERVER_URL          Remotr server base URL (required)
#   REMOTR_ENROLL_TOKEN        One-time enrollment or deployment token (required unless enrolled)
#   REMOTR_DEPLOYMENT_TOKEN    Alias for REMOTR_ENROLL_TOKEN (reusable deployment token)
#   REMOTR_ENROLL_TOKEN_FILE   Path to token file (mode 0600 recommended)
#   REMOTR_CA_FILE             Path to Remotr CA PEM (skip auto-fetch)
#   REMOTR_CA_PEM              Inline CA PEM (written to /etc/remotr/ca.crt)
#   REMOTR_CA_URL              Fetch CA PEM from URL (default: ${REMOTR_SERVER_URL}/v1/ca.pem)
#   REMOTR_CA_FINGERPRINT      Optional sha256 fingerprint pin (openssl x509 -fingerprint -sha256)
#   REMOTR_VERSION             Release tag or version (default: latest)
#   REMOTR_GITHUB_REPO         owner/repo (default: DavidHoenisch/remotr)
#   REMOTR_BIN_DIR             Binary install dir (default: /usr/local/bin)
#   REMOTR_STATE_DIR           Agent credentials (default: /var/lib/remotr)
#   REMOTR_CONFIG_DIR          Config and systemd env (default: /etc/remotr)
#   REMOTR_SYNC_INTERVAL       Sync interval for systemd (default: 30s)
#   REMOTR_YES                 Set to 1 to skip confirmation prompts
#   REMOTR_SKIP_ENROLL         Set to 1 to install binary/systemd only (upgrade path)
#   REMOTR_DEFER_ENROLL        Set to 1 to enroll on first boot via systemd (writes enroll.env)
#   REMOTR_SKIP_SYSTEMD        Set to 1 to skip systemd unit installation
#   REMOTR_ENDPOINT_ID         Optional stable endpoint identifier (hostname-based if unset)
#   REMOTR_FORCE_ENROLL        Set to 1 to pass --force to remotr-agent enroll
#   REMOTR_VERIFY_CHECKSUMS    Set to 1 to verify release checksums.txt

set -euo pipefail

REMOTR_GITHUB_REPO="${REMOTR_GITHUB_REPO:-DavidHoenisch/remotr}"
REMOTR_VERSION="${REMOTR_VERSION:-latest}"
REMOTR_BIN_DIR="${REMOTR_BIN_DIR:-/usr/local/bin}"
REMOTR_STATE_DIR="${REMOTR_STATE_DIR:-/var/lib/remotr}"
REMOTR_CONFIG_DIR="${REMOTR_CONFIG_DIR:-/etc/remotr}"
REMOTR_SYNC_INTERVAL="${REMOTR_SYNC_INTERVAL:-30s}"

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

confirm() {
  if [[ "${REMOTR_YES:-}" == "1" ]]; then
    return 0
  fi

  local prompt=$1 reply
  local tty=/dev/tty

  if [[ ! -r "$tty" ]] || [[ ! -w "$tty" ]]; then
    die "no terminal — use: bash <(curl -fsSL .../install-agent.sh)  OR  REMOTR_YES=1 curl ... | bash"
  fi

  {
    printf '\n'
    printf '%s\n' "$prompt"
    printf 'Type yes to continue: '
  } >"$tty"

  if ! read -r reply <"$tty"; then
    die "could not read from terminal — try: bash <(curl -fsSL .../install-agent.sh)"
  fi

  case "${reply}" in
    y|Y|yes|YES) return 0 ;;
    *) die "aborted" ;;
  esac
}

require_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    die "run as root (agent apply operations require root)"
  fi
}

require_systemd() {
  if ! command -v systemctl >/dev/null 2>&1; then
    die "systemd is required (set REMOTR_SKIP_SYSTEMD=1 to install binary only)"
  fi
}

normalize_server_url() {
  local url="${REMOTR_SERVER_URL%/}"
  if [[ -z "$url" ]]; then
    die "REMOTR_SERVER_URL is required (e.g. https://remotr.example:8443)"
  fi
  case "$url" in
    http://*|https://*) REMOTR_SERVER_URL=$url ;;
    *) die "REMOTR_SERVER_URL must start with http:// or https://" ;;
  esac
}

resolve_enroll_token() {
  if [[ -n "${REMOTR_ENROLL_TOKEN:-}" ]]; then
    printf '%s' "$REMOTR_ENROLL_TOKEN"
    return
  fi
  if [[ -n "${REMOTR_DEPLOYMENT_TOKEN:-}" ]]; then
    printf '%s' "$REMOTR_DEPLOYMENT_TOKEN"
    return
  fi
  if [[ -n "${REMOTR_ENROLL_TOKEN_FILE:-}" ]]; then
    if [[ ! -f "$REMOTR_ENROLL_TOKEN_FILE" ]]; then
      die "REMOTR_ENROLL_TOKEN_FILE not found: $REMOTR_ENROLL_TOKEN_FILE"
    fi
    tr -d ' \n\r' <"$REMOTR_ENROLL_TOKEN_FILE"
    return
  fi
  return 1
}

normalize_fingerprint() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | tr -d ' \n\r' | sed 's/^sha256//;s/^fingerprint=//;s/://g'
}

verify_ca_fingerprint() {
  local ca_path=$1
  [[ -n "${REMOTR_CA_FINGERPRINT:-}" ]] || return 0
  command -v openssl >/dev/null 2>&1 || die "openssl required for REMOTR_CA_FINGERPRINT"

  local expected actual
  expected=$(normalize_fingerprint "$REMOTR_CA_FINGERPRINT")
  actual=$(openssl x509 -in "$ca_path" -noout -fingerprint -sha256 2>/dev/null | sed 's/.*=//')
  actual=$(normalize_fingerprint "$actual")
  if [[ "$actual" != "$expected" ]]; then
    die "CA fingerprint mismatch (expected ${expected}, got ${actual})"
  fi
}

validate_ca_pem() {
  local ca_path=$1
  if command -v openssl >/dev/null 2>&1; then
    openssl x509 -in "$ca_path" -noout >/dev/null 2>&1 || die "invalid CA certificate at $ca_path"
    return
  fi
  grep -q 'BEGIN CERTIFICATE' "$ca_path" || die "invalid CA PEM at $ca_path"
}

finalize_ca_install() {
  local ca_path=$1
  validate_ca_pem "$ca_path"
  verify_ca_fingerprint "$ca_path"
  chmod 0644 "$ca_path"
  REMOTR_CA_PATH=$ca_path
}

fetch_ca_from_server() {
  local ca_path=$1
  local url="${REMOTR_CA_URL:-${REMOTR_SERVER_URL}/v1/ca.pem}"
  local tmp="${ca_path}.download"

  log "fetching Remotr CA from ${url}"
  # Server TLS is signed by this CA; first fetch uses -k (trust-on-first-use via URL the admin sent).
  if ! curl -kfsSL "$url" -o "$tmp"; then
    die "failed to download CA — check REMOTR_SERVER_URL or set REMOTR_CA_FILE"
  fi
  mv "$tmp" "$ca_path"
  finalize_ca_install "$ca_path"
}

install_ca() {
  local ca_path="${REMOTR_CONFIG_DIR}/ca.crt"
  mkdir -p "$REMOTR_CONFIG_DIR"
  chmod 0755 "$REMOTR_CONFIG_DIR"

  if [[ -n "${REMOTR_CA_FILE:-}" ]]; then
    [[ -f "$REMOTR_CA_FILE" ]] || die "REMOTR_CA_FILE not found: $REMOTR_CA_FILE"
    install -m 0644 "$REMOTR_CA_FILE" "$ca_path"
    finalize_ca_install "$ca_path"
    return
  fi

  if [[ -n "${REMOTR_CA_PEM:-}" ]]; then
    printf '%s\n' "$REMOTR_CA_PEM" >"$ca_path"
    finalize_ca_install "$ca_path"
    return
  fi

  if [[ -n "${REMOTR_CA_URL:-}" ]]; then
    curl -fsSL "$REMOTR_CA_URL" -o "$ca_path" 2>/dev/null || curl -kfsSL "$REMOTR_CA_URL" -o "$ca_path"
    finalize_ca_install "$ca_path"
    return
  fi

  if [[ -f "$ca_path" ]]; then
    warn "using existing CA at $ca_path"
    finalize_ca_install "$ca_path"
    return
  fi

  fetch_ca_from_server "$ca_path"
}

detect_arch() {
  local machine
  machine="$(uname -m)"
  case "$machine" in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) die "unsupported architecture: $machine (linux amd64/arm64 only)" ;;
  esac
}

resolve_version() {
  local v="${REMOTR_VERSION}"
  if [[ "$v" == "latest" ]]; then
    if ! command -v jq >/dev/null 2>&1; then
      die "REMOTR_VERSION=latest requires jq, or set REMOTR_VERSION to a release tag (e.g. v1.0.0)"
    fi
    v=$(curl -fsSL "https://api.github.com/repos/${REMOTR_GITHUB_REPO}/releases/latest" | jq -r .tag_name)
    [[ -n "$v" && "$v" != "null" ]] || die "could not resolve latest release for ${REMOTR_GITHUB_REPO}"
  fi
  v="${v#v}"
  printf '%s' "$v"
}

download_agent() {
  local version=$1 arch=$2
  local tag="v${version}"
  local asset="remotr-agent_${version}_linux_${arch}.tar.gz"
  local base="https://github.com/${REMOTR_GITHUB_REPO}/releases/download/${tag}"
  local url="${base}/${asset}"
  local tmp
  tmp=$(mktemp -d)
  # Expand $tmp now: RETURN trap runs after function locals are cleared (breaks under set -u).
  trap "rm -rf '${tmp}'" RETURN

  log "downloading ${url}"
  curl -fsSL "$url" -o "${tmp}/${asset}"

  if [[ "${REMOTR_VERIFY_CHECKSUMS:-}" == "1" ]]; then
    command -v sha256sum >/dev/null 2>&1 || die "sha256sum required for REMOTR_VERIFY_CHECKSUMS=1"
    curl -fsSL "${base}/checksums.txt" -o "${tmp}/checksums.txt"
    (cd "$tmp" && grep -F " ${asset}" checksums.txt | sha256sum -c -)
  fi

  tar -xzf "${tmp}/${asset}" -C "$tmp"
  [[ -f "${tmp}/remotr-agent" ]] || die "archive missing remotr-agent binary"
  install -d -m 0755 "$REMOTR_BIN_DIR"
  install -m 0755 "${tmp}/remotr-agent" "${REMOTR_BIN_DIR}/remotr-agent"
}

wait_for_server() {
  local i
  log "waiting for ${REMOTR_SERVER_URL}/healthz"
  for i in $(seq 1 60); do
    if curl -fsS --cacert "$REMOTR_CA_PATH" "${REMOTR_SERVER_URL}/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  die "server health check failed — check REMOTR_SERVER_URL, firewall, and REMOTR_CA_*"
}

run_enroll() {
  local token=$1
  local force_flag=()
  if [[ "${REMOTR_FORCE_ENROLL:-}" == "1" ]]; then
    force_flag=(--force)
  fi

  if [[ -f "${REMOTR_STATE_DIR}/state.json" ]] && [[ "${REMOTR_FORCE_ENROLL:-}" != "1" ]]; then
    log "already enrolled (${REMOTR_STATE_DIR}/state.json); skipping enroll"
    return 0
  fi

  mkdir -p "$REMOTR_STATE_DIR"
  chmod 0700 "$REMOTR_STATE_DIR"

  local endpoint_args=()
  if [[ -n "${REMOTR_ENDPOINT_ID:-}" ]]; then
    endpoint_args=(--endpoint-id "$REMOTR_ENDPOINT_ID")
  fi

  log "enrolling endpoint with ${REMOTR_SERVER_URL}"
  REMOTR_ENROLL_TOKEN=$token \
    "${REMOTR_BIN_DIR}/remotr-agent" enroll \
      --server-url "$REMOTR_SERVER_URL" \
      --ca "$REMOTR_CA_PATH" \
      --state-dir "$REMOTR_STATE_DIR" \
      --no-sync \
      "${endpoint_args[@]}" \
      "${force_flag[@]}"
}

write_agent_env() {
  local env_file="${REMOTR_CONFIG_DIR}/agent.env"
  install -d -m 0755 "$REMOTR_CONFIG_DIR"
  cat >"$env_file" <<EOF
# Managed by scripts/install-agent.sh — do not put secrets here.
REMOTR_SERVER_URL=${REMOTR_SERVER_URL}
REMOTR_STATE_DIR=${REMOTR_STATE_DIR}
REMOTR_SYNC_INTERVAL=${REMOTR_SYNC_INTERVAL}
REMOTR_TLS_CA=${REMOTR_CA_PATH}
EOF
  chmod 0644 "$env_file"
}

write_enroll_env() {
  local token=$1
  local env_file="${REMOTR_CONFIG_DIR}/enroll.env"
  install -d -m 0755 "$REMOTR_CONFIG_DIR"
  {
    printf '# Managed by scripts/install-agent.sh — enrollment secret; remove after enroll if desired.\n'
    if [[ -n "${REMOTR_ENROLL_TOKEN_FILE:-}" ]]; then
      printf 'REMOTR_ENROLL_TOKEN_FILE=%s\n' "$REMOTR_ENROLL_TOKEN_FILE"
    elif [[ -n "$token" ]]; then
      printf 'REMOTR_ENROLL_TOKEN=%s\n' "$token"
    else
      die "deferred enroll requires REMOTR_ENROLL_TOKEN, REMOTR_DEPLOYMENT_TOKEN, or REMOTR_ENROLL_TOKEN_FILE"
    fi
    if [[ -n "${REMOTR_ENDPOINT_ID:-}" ]]; then
      printf 'REMOTR_ENDPOINT_ID=%s\n' "$REMOTR_ENDPOINT_ID"
    fi
  } >"$env_file"
  chmod 0600 "$env_file"
}

install_systemd_units() {
  local defer_enroll=$1
  local sync_unit="/etc/systemd/system/remotr-agent.service"
  local enroll_unit="/etc/systemd/system/remotr-agent-enroll.service"

  if [[ "$defer_enroll" == "1" ]]; then
    cat >"$sync_unit" <<'EOF'
[Unit]
Description=Remotr endpoint agent
After=network-online.target remotr-agent-enroll.service
Wants=network-online.target remotr-agent-enroll.service
ConditionPathExists=/var/lib/remotr/state.json

[Service]
Type=simple
EnvironmentFile=-/etc/remotr/agent.env
ExecStart=/usr/local/bin/remotr-agent
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
  else
    cat >"$sync_unit" <<'EOF'
[Unit]
Description=Remotr endpoint agent
After=network-online.target
Wants=network-online.target
ConditionPathExists=/var/lib/remotr/state.json

[Service]
Type=simple
EnvironmentFile=-/etc/remotr/agent.env
ExecStart=/usr/local/bin/remotr-agent
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
  fi

  if [[ "$REMOTR_BIN_DIR" != "/usr/local/bin" ]]; then
    sed -i "s|/usr/local/bin/remotr-agent|${REMOTR_BIN_DIR}/remotr-agent|g" "$sync_unit"
  fi
  sed -i "s|/var/lib/remotr/state.json|${REMOTR_STATE_DIR}/state.json|g" "$sync_unit"

  cat >"$enroll_unit" <<'EOF'
[Unit]
Description=Remotr agent enrollment (oneshot)
ConditionPathExists=!/var/lib/remotr/state.json
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
EnvironmentFile=-/etc/remotr/enroll.env
EnvironmentFile=-/etc/remotr/agent.env
ExecStartPre=/bin/sh -c 'for i in $(seq 1 60); do curl -fsS --cacert "${REMOTR_TLS_CA}" "${REMOTR_SERVER_URL}/healthz" >/dev/null 2>&1 && exit 0; sleep 2; done; exit 1'
ExecStart=/usr/local/bin/remotr-agent enroll --no-sync
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

  if [[ "$REMOTR_BIN_DIR" != "/usr/local/bin" ]]; then
    sed -i "s|/usr/local/bin/remotr-agent|${REMOTR_BIN_DIR}/remotr-agent|g" "$enroll_unit"
  fi
  sed -i "s|!/var/lib/remotr/state.json|!${REMOTR_STATE_DIR}/state.json|g" "$enroll_unit"

  systemctl daemon-reload
  systemctl enable remotr-agent.service
  if [[ "$defer_enroll" == "1" ]]; then
    systemctl enable remotr-agent-enroll.service
  fi
}

show_plan() {
  local tty=/dev/tty
  [[ -w "$tty" ]] || return 0
  {
    printf '\n'
    printf 'Remotr agent install plan\n'
    printf '  Server URL:   %s\n' "$REMOTR_SERVER_URL"
    printf '  Version:      %s\n' "$REMOTR_VERSION"
    printf '  Binary:       %s/remotr-agent\n' "$REMOTR_BIN_DIR"
    printf '  State dir:    %s\n' "$REMOTR_STATE_DIR"
    printf '  CA:           %s\n' "${REMOTR_CA_PATH:-<from REMOTR_CA_*>}"
    if [[ "${REMOTR_SKIP_ENROLL:-}" == "1" ]]; then
      printf '  Enroll:       skip\n'
    elif [[ "${REMOTR_DEFER_ENROLL:-}" == "1" ]]; then
      printf '  Enroll:       deferred (first boot)\n'
    else
      printf '  Enroll:       now\n'
    fi
    printf '  systemd:      %s\n' "$( [[ "${REMOTR_SKIP_SYSTEMD:-}" == "1" ]] && echo skip || echo yes )"
    printf '\n'
  } >"$tty"
}

main() {
  require_root
  normalize_server_url
  install_ca

  if [[ "${REMOTR_SKIP_SYSTEMD:-}" != "1" ]]; then
    require_systemd
  fi

  show_plan
  confirm "Install remotr-agent on this machine and enroll with the Remotr server?"

  local arch version token=""
  arch=$(detect_arch)
  version=$(resolve_version)
  log "installing remotr-agent ${version} (linux/${arch})"
  download_agent "$version" "$arch"

  write_agent_env
  wait_for_server

  local defer_enroll=0
  if [[ "${REMOTR_DEFER_ENROLL:-}" == "1" ]]; then
    defer_enroll=1
  fi

  if [[ -f "${REMOTR_STATE_DIR}/state.json" ]] && [[ "${REMOTR_SKIP_ENROLL:-}" == "1" ]]; then
    log "upgrading remotr-agent binary (keeping enrollment in ${REMOTR_STATE_DIR})"
  fi

  if [[ "${REMOTR_SKIP_ENROLL:-}" != "1" ]]; then
    if ! token=$(resolve_enroll_token); then
      if [[ -f "${REMOTR_STATE_DIR}/state.json" ]]; then
        log "no token provided; using existing enrollment"
      elif [[ "$defer_enroll" == "1" ]]; then
        die "deferred enroll requires REMOTR_ENROLL_TOKEN, REMOTR_DEPLOYMENT_TOKEN, or REMOTR_ENROLL_TOKEN_FILE"
      else
        die "enrollment token required: REMOTR_ENROLL_TOKEN, REMOTR_DEPLOYMENT_TOKEN, or REMOTR_ENROLL_TOKEN_FILE"
      fi
    elif [[ "$defer_enroll" == "1" ]]; then
      write_enroll_env "$token"
      log "enrollment deferred to first boot (${REMOTR_CONFIG_DIR}/enroll.env)"
    else
      run_enroll "$token"
    fi
  fi

  if [[ "${REMOTR_SKIP_SYSTEMD:-}" != "1" ]]; then
    install_systemd_units "$defer_enroll"
    if [[ "$defer_enroll" == "1" ]] && [[ ! -f "${REMOTR_STATE_DIR}/state.json" ]]; then
      systemctl start remotr-agent-enroll.service 2>/dev/null || true
      log "started remotr-agent-enroll.service"
    fi
    if [[ -f "${REMOTR_STATE_DIR}/state.json" ]]; then
      systemctl restart remotr-agent.service 2>/dev/null || systemctl start remotr-agent.service 2>/dev/null || true
      log "enabled remotr-agent.service"
    elif [[ "$defer_enroll" != "1" ]]; then
      systemctl restart remotr-agent.service 2>/dev/null || systemctl start remotr-agent.service 2>/dev/null || true
      log "enabled remotr-agent.service"
    fi
  fi

  if [[ -f "${REMOTR_STATE_DIR}/state.json" ]]; then
    log "endpoint id: $(sed -n 's/.*"endpoint_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${REMOTR_STATE_DIR}/state.json" | head -1)"
  fi

  log "done"
  printf '\n'
  printf 'Agent sync: systemctl status remotr-agent\n'
  printf 'Re-enroll:  remotr-agent enroll --force --server-url %s --ca %s\n' "$REMOTR_SERVER_URL" "$REMOTR_CA_PATH"
}

main "$@"
