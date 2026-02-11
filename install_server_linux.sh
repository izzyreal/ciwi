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
  echo "sudo is required for server installation" >&2
  exit 1
fi

echo "Requesting sudo access..."
sudo -v

REPO="izzyreal/ciwi"
SERVICE_NAME="ciwi"
UPDATER_SERVICE_NAME="ciwi-updater"
BINARY_PATH="/usr/local/bin/ciwi"
DATA_DIR="/var/lib/ciwi"
ARTIFACTS_DIR="${DATA_DIR}/artifacts"
UPDATES_DIR="${DATA_DIR}/updates"
LOG_DIR="/var/log/ciwi"
ENV_FILE="/etc/default/ciwi"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
UPDATER_UNIT_FILE="/etc/systemd/system/${UPDATER_SERVICE_NAME}.service"
POLKIT_RULE_FILE="/etc/polkit-1/rules.d/90-ciwi-updater.rules"
LOGROTATE_FILE="/etc/logrotate.d/ciwi"
SYSTEMCTL_PATH="$(command -v systemctl)"

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

TMP_DIR="$(mktemp -d -t ciwi-server-install.XXXXXX)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

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
if ! id ciwi >/dev/null 2>&1; then
  sudo useradd --system --home "${DATA_DIR}" --create-home --shell /usr/sbin/nologin ciwi 2>/dev/null || \
  sudo useradd --system --home "${DATA_DIR}" --create-home --shell /bin/false ciwi
fi
sudo mkdir -p "${DATA_DIR}" "${ARTIFACTS_DIR}" "${UPDATES_DIR}" "${LOG_DIR}"
sudo chown -R ciwi:ciwi "${DATA_DIR}" "${LOG_DIR}"

echo "[5/7] Writing config and systemd units..."
if [ ! -f "${ENV_FILE}" ]; then
  sudo tee "${ENV_FILE}" >/dev/null <<EOF
CIWI_SERVER_ADDR=0.0.0.0:8112
CIWI_DB_PATH=${DATA_DIR}/ciwi.db
CIWI_ARTIFACTS_DIR=${ARTIFACTS_DIR}
CIWI_UPDATE_STAGING_DIR=${UPDATES_DIR}
CIWI_UPDATE_STAGED_MANIFEST=${UPDATES_DIR}/pending.json
CIWI_UPDATER_UNIT=${UPDATER_SERVICE_NAME}.service
CIWI_SYSTEMCTL_PATH=${SYSTEMCTL_PATH}
CIWI_LOG_LEVEL=info
EOF
else
  echo "Keeping existing ${ENV_FILE}"
fi

sudo tee "${UNIT_FILE}" >/dev/null <<EOF
[Unit]
Description=ciwi server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ciwi
Group=ciwi
EnvironmentFile=-${ENV_FILE}
ExecStart=${BINARY_PATH} server
Restart=always
RestartSec=2
WorkingDirectory=${DATA_DIR}
StandardOutput=append:${LOG_DIR}/server.out.log
StandardError=append:${LOG_DIR}/server.err.log

[Install]
WantedBy=multi-user.target
EOF

sudo tee "${UPDATER_UNIT_FILE}" >/dev/null <<EOF
[Unit]
Description=ciwi staged updater
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
EnvironmentFile=-${ENV_FILE}
ExecStart=${BINARY_PATH} apply-staged-update
WorkingDirectory=${DATA_DIR}
NoNewPrivileges=true
EOF

echo "[6/7] Writing polkit rule for updater trigger..."
sudo mkdir -p "/etc/polkit-1/rules.d"
sudo tee "${POLKIT_RULE_FILE}" >/dev/null <<EOF
polkit.addRule(function(action, subject) {
  if (action.id == "org.freedesktop.systemd1.manage-units" &&
      action.lookup("unit") == "${UPDATER_SERVICE_NAME}.service" &&
      action.lookup("verb") == "start" &&
      subject.user == "ciwi") {
    return polkit.Result.YES;
  }
});
EOF
sudo chmod 0644 "${POLKIT_RULE_FILE}"

echo "[6.5/7] Writing logrotate policy..."
sudo tee "${LOGROTATE_FILE}" >/dev/null <<EOF
${LOG_DIR}/server.out.log ${LOG_DIR}/server.err.log {
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

echo "[7/7] Starting service..."
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}"

echo
echo "ciwi server installed and started."
echo "Service:      ${SERVICE_NAME}"
echo "Binary:       ${BINARY_PATH}"
echo "Config:       ${ENV_FILE}"
echo "Data:         ${DATA_DIR}"
echo "Artifacts:    ${ARTIFACTS_DIR}"
echo "Updates:      ${UPDATES_DIR}"
echo
echo "Useful commands:"
echo "  sudo systemctl status ${SERVICE_NAME}"
echo "  sudo journalctl -u ${SERVICE_NAME} -f"
echo "  curl -s http://127.0.0.1:8112/healthz"
echo
echo "Open UI:"
echo "  http://<server-host>:8112/"
echo
echo "Uninstall:"
echo "  sudo systemctl disable --now ${SERVICE_NAME}"
echo "  sudo rm -f ${UNIT_FILE} ${BINARY_PATH}"
echo "  sudo systemctl daemon-reload"
