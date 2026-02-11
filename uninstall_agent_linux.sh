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
  echo "sudo is required for agent uninstall" >&2
  exit 1
fi

echo "Requesting sudo access..."
sudo -v

SERVICE_NAME="ciwi-agent"
UNIT_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
ENV_FILE="/etc/default/ciwi-agent"
LOGROTATE_FILE="/etc/logrotate.d/ciwi-agent"
DATA_DIR="/var/lib/ciwi-agent"
LOG_DIR="/var/log/ciwi-agent"
USER_NAME="ciwi-agent"

echo "[1/4] Stopping and disabling service..."
sudo systemctl disable --now "${SERVICE_NAME}" >/dev/null 2>&1 || true

echo "[2/4] Removing unit and env file..."
sudo rm -f "${UNIT_FILE}" "${ENV_FILE}" "${LOGROTATE_FILE}"
sudo systemctl daemon-reload

echo "[3/4] Keeping binary and data by default:"
echo "  /usr/local/bin/ciwi"
echo "  ${DATA_DIR}"
echo "  ${LOG_DIR}"
printf "Remove data, logs and ciwi-agent user too? [y/N]: "
read -r ANSWER
case "${ANSWER}" in
  y|Y|yes|YES)
    sudo rm -rf "${DATA_DIR}" "${LOG_DIR}"
    sudo userdel "${USER_NAME}" >/dev/null 2>&1 || true
    echo "Removed ${DATA_DIR}, ${LOG_DIR}, and user ${USER_NAME}"
    ;;
  *)
    echo "Kept ${DATA_DIR}, ${LOG_DIR}, user ${USER_NAME}"
    ;;
esac

echo "[4/4] Done."
echo "ciwi Linux agent uninstall complete."
