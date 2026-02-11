#!/bin/sh
set -eu

if [ "$#" -ne 0 ]; then
  echo "this uninstaller takes no options; run it directly" >&2
  exit 2
fi

if [ "$(uname -s)" != "Linux" ]; then
  echo "this uninstaller is for Linux only" >&2
  exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required (systemd-based distro)" >&2
  exit 1
fi

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required for server uninstall" >&2
  exit 1
fi

echo "Requesting sudo access..."
sudo -v

SERVICE_NAME="ciwi"
UPDATER_SERVICE_NAME="ciwi-updater"
BINARY_PATH="/usr/local/bin/ciwi"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
UPDATER_UNIT_FILE="/etc/systemd/system/${UPDATER_SERVICE_NAME}.service"
ENV_FILE="/etc/default/ciwi"
POLKIT_RULE_FILE="/etc/polkit-1/rules.d/90-ciwi-updater.rules"
LOGROTATE_FILE="/etc/logrotate.d/ciwi"
DATA_DIR="/var/lib/ciwi"
LOG_DIR="/var/log/ciwi"

echo "[1/5] Stopping and disabling service..."
sudo systemctl disable --now "${SERVICE_NAME}" >/dev/null 2>&1 || true

echo "[2/5] Removing systemd unit..."
sudo rm -f "${UNIT_FILE}"
sudo rm -f "${UPDATER_UNIT_FILE}"
sudo systemctl daemon-reload

echo "[3/5] Removing ciwi binary..."
sudo rm -f "${BINARY_PATH}"

echo "[4/5] Removing default env file..."
sudo rm -f "${ENV_FILE}"
sudo rm -f "${POLKIT_RULE_FILE}"
sudo rm -f "${LOGROTATE_FILE}"

echo "[5/5] Keeping data and logs by default:"
echo "  ${DATA_DIR}"
echo "  ${LOG_DIR}"
printf "Remove data and logs too? [y/N]: "
read -r ANSWER
case "${ANSWER}" in
  y|Y|yes|YES)
    sudo rm -rf "${DATA_DIR}" "${LOG_DIR}"
    echo "Removed ${DATA_DIR} and ${LOG_DIR}"
    ;;
  *)
    echo "Kept ${DATA_DIR} and ${LOG_DIR}"
    ;;
esac

echo
echo "ciwi Linux server uninstall complete."
