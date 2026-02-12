#!/bin/sh
set -eu

if [ "$#" -ne 0 ]; then
  echo "this installer takes no options; run it directly" >&2
  exit 2
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

trim_single_line() {
  printf '%s' "$1" | tr -d '\r\n' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

github_auth_header() {
  token="$(trim_single_line "${CIWI_GITHUB_TOKEN:-}")"
  if [ -z "$token" ]; then
    token="$(trim_single_line "${INSTALL_GITHUB_TOKEN:-}")"
  fi
  if [ -n "$token" ]; then
    printf 'Authorization: Bearer %s\n' "$token"
  else
    printf '%s\n' ""
  fi
}

github_api_get() {
  url="$1"
  auth_header="$(github_auth_header)"
  if [ -n "$auth_header" ]; then
    curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: ciwi-installer" -H "$auth_header" "$url"
  else
    curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: ciwi-installer" "$url"
  fi
}

fetch_latest_tag() {
  api_url="https://api.github.com/repos/${REPO}/releases/latest"
  github_api_get "$api_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p'
}

if [ "$(uname -s)" != "Linux" ]; then
  echo "this installer is for Linux only" >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required (systemd-based distro)" >&2
  exit 1
fi

require_cmd curl
require_cmd sha256sum
require_cmd install
require_cmd awk
require_cmd sed

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required for agent installation" >&2
  exit 1
fi

echo "Requesting sudo access..."
sudo -v

REPO="izzyreal/ciwi"
SERVICE_NAME="ciwi-agent"
BINARY_PATH="/usr/local/bin/ciwi"
USER_NAME="ciwi-agent"
DATA_DIR="/var/lib/ciwi-agent"
WORK_DIR="${DATA_DIR}/work"
LOG_DIR="/var/log/ciwi-agent"
ENV_FILE="/etc/default/ciwi-agent"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
LOGROTATE_FILE="/etc/logrotate.d/ciwi-agent"

SERVER_URL="${CIWI_SERVER_URL:-http://127.0.0.1:8112}"
AGENT_ID="${CIWI_AGENT_ID:-agent-$(hostname -s)}"

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64) GOARCH="amd64" ;;
  arm64|aarch64) GOARCH="arm64" ;;
  *)
    echo "unsupported architecture: $ARCH_RAW" >&2
    exit 1
    ;;
esac

ASSET="ciwi-linux-${GOARCH}"
CHECKSUM_ASSET="ciwi-checksums.txt"
RELEASE_BASE="https://github.com/${REPO}/releases/latest/download"

TMP_DIR="$(mktemp -d -t ciwi-agent-install.XXXXXX)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

TARGET_VERSION="$(fetch_latest_tag 2>/dev/null || true)"
if [ -n "$TARGET_VERSION" ]; then
  echo "[info] Preparing to install ciwi agent version: ${TARGET_VERSION}"
else
  echo "[info] Preparing to install ciwi agent version: unknown (GitHub tag query failed)"
fi

echo "[1/6] Downloading ${ASSET}..."
curl -fsSL "${RELEASE_BASE}/${ASSET}" -o "${TMP_DIR}/${ASSET}"
curl -fsSL "${RELEASE_BASE}/${CHECKSUM_ASSET}" -o "${TMP_DIR}/${CHECKSUM_ASSET}"

echo "[2/6] Verifying checksum..."
EXPECTED_SHA="$(awk -v n="${ASSET}" '
  $0 ~ /^[[:space:]]*#/ { next }
  NF >= 2 {
    name=$2
    sub(/^\*/, "", name)
    base=name
    sub(/^.*\//, "", base)
    if (name == n || base == n) { print tolower($1); exit }
  }
' "${TMP_DIR}/${CHECKSUM_ASSET}")"
if [ -z "$EXPECTED_SHA" ]; then
  echo "checksum entry not found for ${ASSET} in ${CHECKSUM_ASSET}" >&2
  exit 1
fi
ACTUAL_SHA="$(sha256sum "${TMP_DIR}/${ASSET}" | awk '{print tolower($1)}')"
if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
  echo "checksum mismatch for ${ASSET}" >&2
  echo "expected: $EXPECTED_SHA" >&2
  echo "actual:   $ACTUAL_SHA" >&2
  exit 1
fi

echo "[3/6] Installing ciwi binary..."
sudo install -m 0755 "${TMP_DIR}/${ASSET}" "${BINARY_PATH}"

echo "[4/6] Preparing service user and directories..."
if ! id "${USER_NAME}" >/dev/null 2>&1; then
  sudo useradd --system --home "${DATA_DIR}" --create-home --shell /usr/sbin/nologin "${USER_NAME}" 2>/dev/null || \
  sudo useradd --system --home "${DATA_DIR}" --create-home --shell /bin/false "${USER_NAME}"
fi
sudo mkdir -p "${DATA_DIR}" "${WORK_DIR}" "${LOG_DIR}"
sudo chown -R "${USER_NAME}:${USER_NAME}" "${DATA_DIR}" "${LOG_DIR}"

echo "[5/6] Writing config and systemd unit..."
if [ ! -f "${ENV_FILE}" ]; then
  sudo tee "${ENV_FILE}" >/dev/null <<EOF
CIWI_SERVER_URL=${SERVER_URL}
CIWI_AGENT_ID=${AGENT_ID}
CIWI_AGENT_WORKDIR=${WORK_DIR}
CIWI_LOG_LEVEL=info
EOF
else
  echo "Keeping existing ${ENV_FILE}"
fi

sudo tee "${UNIT_FILE}" >/dev/null <<EOF
[Unit]
Description=ciwi agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${USER_NAME}
Group=${USER_NAME}
EnvironmentFile=-${ENV_FILE}
ExecStart=${BINARY_PATH} agent
Restart=always
RestartSec=2
WorkingDirectory=${DATA_DIR}
StandardOutput=append:${LOG_DIR}/agent.out.log
StandardError=append:${LOG_DIR}/agent.err.log

[Install]
WantedBy=multi-user.target
EOF

echo "[5.5/6] Writing logrotate policy..."
sudo tee "${LOGROTATE_FILE}" >/dev/null <<EOF
${LOG_DIR}/agent.out.log ${LOG_DIR}/agent.err.log {
  size 100M
  rotate 3
  missingok
  notifempty
  compress
  delaycompress
  copytruncate
}
EOF
sudo chmod 0644 "${LOGROTATE_FILE}"

echo "[6/6] Starting service..."
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}"

echo
echo "ciwi agent installed and started."
echo "Service:      ${SERVICE_NAME}"
echo "Binary:       ${BINARY_PATH}"
echo "Config:       ${ENV_FILE}"
echo "Data:         ${DATA_DIR}"
echo "Workdir:      ${WORK_DIR}"
echo "Logs:         ${LOG_DIR}"
echo
echo "Useful commands:"
echo "  sudo systemctl status ${SERVICE_NAME}"
echo "  sudo journalctl -u ${SERVICE_NAME} -f"
echo "  sudo tail -f ${LOG_DIR}/agent.out.log ${LOG_DIR}/agent.err.log"
echo
echo "To change target server:"
echo "  sudoedit ${ENV_FILE}"
echo "  sudo systemctl restart ${SERVICE_NAME}"
