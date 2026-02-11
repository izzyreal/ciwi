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

probe_server() {
  url="$1"
  health="$(curl -fsS --max-time 1 "${url}/healthz" 2>/dev/null || true)"
  info="$(curl -fsS --max-time 1 "${url}/api/v1/server-info" 2>/dev/null || true)"
  case "$health" in
    *'"status":"ok"'*|*'"status": "ok"'*) ;;
    *) return 1 ;;
  esac
  case "$info" in
    *'"name":"ciwi"'*|*'"name": "ciwi"'*) ;;
    *) return 1 ;;
  esac
  case "$info" in
    *'"api_version":1'*|*'"api_version": 1'*) return 0 ;;
    *) return 1 ;;
  esac
}

append_unique() {
  list="$1"
  item="$2"
  if [ -z "$list" ]; then
    printf '%s\n' "$item"
    return
  fi
  if printf '%s\n' "$list" | grep -Fxq "$item"; then
    printf '%s\n' "$list"
    return
  fi
  printf '%s\n%s\n' "$list" "$item"
}

discover_servers() {
  found=""
  if probe_server "http://127.0.0.1:8112"; then
    found="$(append_unique "$found" "http://127.0.0.1:8112")"
  fi
  if probe_server "http://localhost:8112"; then
    found="$(append_unique "$found" "http://localhost:8112")"
  fi

  if command -v dns-sd >/dev/null 2>&1; then
    browse_tmp="$(mktemp -t ciwi-mdns-browse.XXXXXX)"
    dns-sd -B _ciwi._tcp local >"$browse_tmp" 2>/dev/null &
    browse_pid=$!
    sleep 2
    kill "$browse_pid" >/dev/null 2>&1 || true
    wait "$browse_pid" >/dev/null 2>&1 || true

    for name in $(awk '/Add/ {print $NF}' "$browse_tmp" | sort -u); do
      resolve_tmp="$(mktemp -t ciwi-mdns-resolve.XXXXXX)"
      dns-sd -L "$name" _ciwi._tcp local >"$resolve_tmp" 2>/dev/null &
      resolve_pid=$!
      sleep 2
      kill "$resolve_pid" >/dev/null 2>&1 || true
      wait "$resolve_pid" >/dev/null 2>&1 || true

      endpoint="$(awk '
        /can be reached at/ {
          line=$0
          sub(/.*can be reached at /, "", line)
          sub(/\.$/, "", line)
          split(line, a, ":")
          if (length(a) >= 2) {
            host=a[1]
            port=a[2]
            gsub(/[[:space:]]/, "", host)
            gsub(/[[:space:]]/, "", port)
            print "http://" host ":" port
            exit
          }
        }
      ' "$resolve_tmp")"
      rm -f "$resolve_tmp"

      if [ -n "$endpoint" ] && probe_server "$endpoint"; then
        found="$(append_unique "$found" "$endpoint")"
      fi
    done
    rm -f "$browse_tmp"
  fi

  if command -v arp >/dev/null 2>&1; then
    for ip in $(arp -an | awk '{print $2}' | tr -d '()' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -u); do
      url="http://${ip}:8112"
      if probe_server "$url"; then
        found="$(append_unique "$found" "$url")"
      fi
    done
  fi

  printf '%s\n' "$found" | sed '/^$/d'
}

choose_server_url() {
  discovered="$(discover_servers)"
  count="$(printf '%s\n' "$discovered" | sed '/^$/d' | wc -l | tr -d ' ')"

  if [ "$count" -eq 1 ]; then
    printf '%s\n' "$discovered" | sed -n '1p'
    return
  fi

  if [ "$count" -gt 1 ]; then
    echo "Multiple ciwi servers discovered:"
    i=0
    printf '%s\n' "$discovered" | while IFS= read -r url; do
      i=$((i + 1))
      echo "  [$i] $url"
    done
    printf "Choose server number [1]: "
    read -r choice
    if [ -z "$choice" ]; then
      choice="1"
    fi
    selected="$(printf '%s\n' "$discovered" | sed -n "${choice}p")"
    if [ -n "$selected" ]; then
      printf '%s\n' "$selected"
      return
    fi
    echo "invalid selection" >&2
    exit 1
  fi

  printf "No ciwi server auto-discovered. Enter server URL (example http://bhakti.local:8112): "
  read -r entered
  entered="$(printf '%s' "$entered" | tr -d '[:space:]')"
  if [ -z "$entered" ]; then
    echo "server URL is required" >&2
    exit 1
  fi
  printf '%s\n' "$entered"
}

install_binary() {
  src="$1"
  default_dir="/usr/local/bin"
  fallback_dir="$HOME/.local/bin"

  if [ -w "$default_dir" ] || [ ! -e "$default_dir" ] && [ -w "/usr/local" ]; then
    mkdir -p "$default_dir"
    install -m 0755 "$src" "${default_dir}/ciwi"
    printf '%s\n' "$default_dir"
    return
  fi

  printf "Install to /usr/local/bin requires sudo. Continue? [Y/n]: "
  read -r answer
  case "$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]')" in
    ""|y|yes)
      sudo mkdir -p "$default_dir"
      sudo install -m 0755 "$src" "${default_dir}/ciwi"
      printf '%s\n' "$default_dir"
      return
      ;;
  esac

  mkdir -p "$fallback_dir"
  install -m 0755 "$src" "${fallback_dir}/ciwi"
  printf '%s\n' "$fallback_dir"
}

if [ "$(uname -s)" != "Darwin" ]; then
  echo "this installer is for macOS only" >&2
  exit 1
fi
if [ "$(id -u)" -eq 0 ]; then
  echo "run as your normal user (not root); this installs a LaunchAgent" >&2
  exit 1
fi

require_cmd curl
require_cmd shasum
require_cmd launchctl
require_cmd install

REPO="izzyreal/ciwi"
LABEL="nl.izmar.ciwi.agent"
LOG_DIR="$HOME/Library/Logs/ciwi"
PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"
WORKDIR="$HOME/.ciwi-agent"
HOST_NAME="$(scutil --get LocalHostName 2>/dev/null || hostname)"
AGENT_ID="agent-${HOST_NAME}"
SERVER_URL="$(choose_server_url)"

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  arm64|aarch64) GOARCH="arm64" ;;
  x86_64|amd64) GOARCH="amd64" ;;
  *)
    echo "unsupported architecture: $ARCH_RAW" >&2
    exit 1
    ;;
esac

ASSET="ciwi-darwin-${GOARCH}"
CHECKSUM_ASSET="ciwi-checksums.txt"
RELEASE_BASE="https://github.com/${REPO}/releases/latest/download"

TMP_DIR="$(mktemp -d -t ciwi-agent-install.XXXXXX)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

echo "[1/5] Downloading ${ASSET} from ${REPO}..."
curl -fsSL "${RELEASE_BASE}/${ASSET}" -o "${TMP_DIR}/${ASSET}"
curl -fsSL "${RELEASE_BASE}/${CHECKSUM_ASSET}" -o "${TMP_DIR}/${CHECKSUM_ASSET}"

echo "[2/5] Verifying checksum..."
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
ACTUAL_SHA="$(shasum -a 256 "${TMP_DIR}/${ASSET}" | awk '{print tolower($1)}')"
if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
  echo "checksum mismatch for ${ASSET}" >&2
  echo "expected: $EXPECTED_SHA" >&2
  echo "actual:   $ACTUAL_SHA" >&2
  exit 1
fi

echo "[3/5] Installing binary..."
mkdir -p "$WORKDIR" "$LOG_DIR" "$HOME/Library/LaunchAgents"
INSTALL_DIR="$(install_binary "${TMP_DIR}/${ASSET}")"

echo "[4/5] Writing LaunchAgent plist..."
cat >"$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/ciwi</string>
    <string>agent</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>CIWI_SERVER_URL</key>
    <string>${SERVER_URL}</string>
    <key>CIWI_AGENT_ID</key>
    <string>${AGENT_ID}</string>
    <key>CIWI_AGENT_WORKDIR</key>
    <string>${WORKDIR}</string>
    <key>PATH</key>
    <string>${INSTALL_DIR}:/usr/local/go/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/agent.out.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/agent.err.log</string>
</dict>
</plist>
EOF

echo "[5/5] Bootstrapping LaunchAgent..."
UID_NUM="$(id -u)"
launchctl bootout "gui/${UID_NUM}" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl bootstrap "gui/${UID_NUM}" "$PLIST_PATH"
launchctl enable "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true
launchctl kickstart -k "gui/${UID_NUM}/${LABEL}"

echo
echo "ciwi agent installed and started."
echo "Label:       ${LABEL}"
echo "Binary:      ${INSTALL_DIR}/ciwi"
echo "Plist:       ${PLIST_PATH}"
echo "Server URL:  ${SERVER_URL}"
echo "Agent ID:    ${AGENT_ID}"
echo "Workdir:     ${WORKDIR}"
echo "Logs:"
echo "  tail -f ${LOG_DIR}/agent.out.log ${LOG_DIR}/agent.err.log"
echo
echo "To uninstall:"
echo "  launchctl bootout gui/\$(id -u) ${PLIST_PATH} || true"
echo "  rm -f ${PLIST_PATH} ${INSTALL_DIR}/ciwi"
