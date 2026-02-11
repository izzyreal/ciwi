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

xml_escape() {
  printf '%s' "$1" \
    | sed -e 's/&/\&amp;/g' \
          -e 's/</\&lt;/g' \
          -e 's/>/\&gt;/g' \
          -e 's/"/\&quot;/g' \
          -e "s/'/\&apos;/g"
}

read_existing_github_token() {
  plist_path="$1"
  if [ ! -f "$plist_path" ]; then
    printf '%s' ""
    return
  fi
  if [ -x /usr/libexec/PlistBuddy ]; then
    /usr/libexec/PlistBuddy -c "Print :EnvironmentVariables:CIWI_GITHUB_TOKEN" "$plist_path" 2>/dev/null || true
    return
  fi
  printf '%s' ""
}

fetch_latest_tag() {
  api_url="https://api.github.com/repos/${REPO}/releases/latest"
  auth_header=""
  token="$(trim_single_line "${CIWI_GITHUB_TOKEN:-}")"
  if [ -z "$token" ]; then
    token="$(trim_single_line "${INSTALL_GITHUB_TOKEN:-}")"
  fi
  if [ -n "$token" ]; then
    auth_header="Authorization: Bearer ${token}"
  fi
  if [ -n "$auth_header" ]; then
    curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: ciwi-installer" -H "$auth_header" "$api_url" \
      | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p'
  else
    curl -fsSL -H "Accept: application/vnd.github+json" -H "User-Agent: ciwi-installer" "$api_url" \
      | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p'
  fi
}

normalize_host() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed 's/\.$//'
}

is_ipv4() {
  printf '%s' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$'
}

resolve_hostname_for_ip() {
  ip="$1"
  if ! is_ipv4 "$ip"; then
    printf '%s\n' ""
    return
  fi
  if command -v dig >/dev/null 2>&1; then
    name="$(dig +short -x "$ip" 2>/dev/null | sed 's/\.$//' | sed -n '1p' || true)"
    if [ -n "$name" ]; then
      printf '%s\n' "$name"
      return
    fi
  fi
  if command -v dscacheutil >/dev/null 2>&1; then
    name="$(dscacheutil -q host -a ip_address "$ip" 2>/dev/null | awk '/^name:/{print $2; exit}' || true)"
    if [ -n "$name" ]; then
      printf '%s\n' "$name"
      return
    fi
  fi
  printf '%s\n' ""
}

canonicalize_url() {
  url="$(printf '%s' "$1" | tr -d '[:space:]')"
  hostport="${url#http://}"
  if [ "$hostport" = "$url" ]; then
    printf '%s\n' "$url"
    return
  fi
  host="${hostport%%:*}"
  port="${hostport##*:}"
  host="$(normalize_host "$host")"
  case "$host" in
    ''|localhost|127.0.0.1) ;;
    *.*) ;;
    *)
      printf 'http://%s.local:%s\n' "$host" "$port"
      return
      ;;
  esac
  if is_ipv4 "$host"; then
    resolved="$(resolve_hostname_for_ip "$host")"
    if [ -n "$resolved" ]; then
      host="$(normalize_host "$resolved")"
      case "$host" in
        *.*) ;;
        *)
          printf 'http://%s.local:%s\n' "$host" "$port"
          return
          ;;
      esac
    fi
  fi
  printf 'http://%s:%s\n' "$host" "$port"
}

normalize_url() {
  url="$(canonicalize_url "$1")"
  hostport="${url#http://}"
  host="${hostport%%:*}"
  port="${hostport##*:}"
  host="$(normalize_host "$host")"
  printf 'http://%s:%s\n' "$host" "$port"
}

probe_server() {
  url="$1"
  health="$(curl -fsS --max-time 2 "${url}/healthz" 2>/dev/null || true)"
  info="$(curl -fsS --max-time 2 "${url}/api/v1/server-info" 2>/dev/null || true)"
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

server_info_json() {
  url="$1"
  curl -fsS --max-time 1 "${url}/api/v1/server-info" 2>/dev/null || true
}

server_hostname_from_info() {
  info="$1"
  printf '%s' "$info" | sed -n 's/.*"hostname"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sed -n '1p'
}

prefer_hostname_url() {
  url="$(canonicalize_url "$1")"
  hostport="${url#http://}"
  host="${hostport%%:*}"
  port="${hostport##*:}"
  if ! is_ipv4 "$host"; then
    printf '%s\n' "$url"
    return
  fi
  info="$(server_info_json "$url")"
  h="$(normalize_host "$(server_hostname_from_info "$info")")"
  if [ -n "$h" ]; then
    if [ "$h" = "localhost" ] || [ "$h" = "127.0.0.1" ]; then
      printf '%s\n' "$url"
      return
    fi
    candidate="http://${h}:${port}"
    if probe_server "$candidate"; then
      printf '%s\n' "$candidate"
      return
    fi
    case "$h" in
      *.*) ;;
      *)
        candidate_local="http://${h}.local:${port}"
        if probe_server "$candidate_local"; then
          printf '%s\n' "$candidate_local"
          return
        fi
        ;;
    esac
  fi
  printf '%s\n' "$url"
}

append_unique() {
  list="$1"
  item="$2"
  item="$(prefer_hostname_url "$item")"
  norm_item="$(normalize_url "$item")"
  if [ -z "$list" ]; then
    printf '%s\n' "$item"
    return
  fi
  while IFS= read -r existing; do
    [ -n "$existing" ] || continue
    if [ "$(normalize_url "$existing")" = "$norm_item" ]; then
      printf '%s\n' "$list"
      return
    fi
  done <<EOF
$list
EOF
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
      host="$(resolve_hostname_for_ip "$ip")"
      if [ -n "$host" ]; then
        url="http://${host}:8112"
      else
        url="http://${ip}:8112"
      fi
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
    echo "Multiple ciwi servers discovered:" >&2
    i=0
    printf '%s\n' "$discovered" | while IFS= read -r url; do
      i=$((i + 1))
      echo "  [$i] $url" >&2
    done
    printf "Choose server number [1]: " >&2
    read -r choice </dev/tty
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

  printf "No ciwi server auto-discovered. Enter server URL (example http://bhakti.local:8112): " >&2
  read -r entered </dev/tty
  entered="$(printf '%s' "$entered" | tr -d '[:space:]')"
  if [ -z "$entered" ]; then
    echo "server URL is required" >&2
    exit 1
  fi
  entered="$(canonicalize_url "$entered")"
  printf '%s\n' "$(prefer_hostname_url "$entered")"
}

install_binary() {
  src="$1"
  target_dir="$HOME/.local/bin"
  # Keep agent binary user-writable so ciwi self-update can replace it in-place.
  mkdir -p "$target_dir"
  install -m 0755 "$src" "${target_dir}/ciwi"
  printf '%s\n' "$target_dir"
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
UPDATER_LABEL="nl.izmar.ciwi.agent-updater"
LOG_DIR="$HOME/Library/Logs/ciwi"
PLIST_PATH="$HOME/Library/LaunchAgents/${LABEL}.plist"
UPDATER_PLIST_PATH="$HOME/Library/LaunchAgents/${UPDATER_LABEL}.plist"
WORKDIR="$HOME/.ciwi-agent"
UPDATES_DIR="$WORKDIR/updates"
MANIFEST_PATH="$UPDATES_DIR/pending.json"
NEWSYSLOG_FILE="/etc/newsyslog.d/ciwi-$(id -un).conf"
HOST_NAME="$(scutil --get LocalHostName 2>/dev/null || hostname)"
AGENT_ID="agent-${HOST_NAME}"
SERVER_URL="$(choose_server_url)"
INSTALL_GITHUB_TOKEN="$(trim_single_line "${CIWI_GITHUB_TOKEN:-}")"
TOKEN_SOURCE="none"
if [ -n "$INSTALL_GITHUB_TOKEN" ]; then
  TOKEN_SOURCE="env"
else
  INSTALL_GITHUB_TOKEN="$(trim_single_line "$(read_existing_github_token "$PLIST_PATH")")"
  if [ -n "$INSTALL_GITHUB_TOKEN" ]; then
    TOKEN_SOURCE="existing-plist"
  fi
fi
INSTALL_GITHUB_TOKEN_ESCAPED="$(xml_escape "$INSTALL_GITHUB_TOKEN")"
GITHUB_TOKEN_ENV_BLOCK=""
if [ -n "$INSTALL_GITHUB_TOKEN_ESCAPED" ]; then
  GITHUB_TOKEN_ENV_BLOCK="$(cat <<EOF
    <key>CIWI_GITHUB_TOKEN</key>
    <string>${INSTALL_GITHUB_TOKEN_ESCAPED}</string>
EOF
)"
fi

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

TARGET_VERSION="$(fetch_latest_tag 2>/dev/null || true)"
if [ -n "$TARGET_VERSION" ]; then
  echo "[info] Preparing to install ciwi agent version: ${TARGET_VERSION}"
else
  echo "[info] Preparing to install ciwi agent version: unknown (GitHub tag query failed)"
fi

echo "[1/6] Downloading ${ASSET} from ${REPO}..."
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
ACTUAL_SHA="$(shasum -a 256 "${TMP_DIR}/${ASSET}" | awk '{print tolower($1)}')"
if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
  echo "checksum mismatch for ${ASSET}" >&2
  echo "expected: $EXPECTED_SHA" >&2
  echo "actual:   $ACTUAL_SHA" >&2
  exit 1
fi

echo "[3/6] Installing binary..."
mkdir -p "$WORKDIR" "$UPDATES_DIR" "$LOG_DIR" "$HOME/Library/LaunchAgents"
INSTALL_DIR="$(install_binary "${TMP_DIR}/${ASSET}")"

echo "[3.5/6] Configuring 100MB log caps (newsyslog)..."
if command -v sudo >/dev/null 2>&1; then
  if sudo -n true >/dev/null 2>&1 || sudo -v >/dev/null 2>&1; then
    sudo tee "$NEWSYSLOG_FILE" >/dev/null <<EOF
${LOG_DIR}/agent.out.log  644  3  102400  *  Z
${LOG_DIR}/agent.err.log  644  3  102400  *  Z
${LOG_DIR}/agent-updater.out.log  644  3  102400  *  Z
${LOG_DIR}/agent-updater.err.log  644  3  102400  *  Z
${LOG_DIR}/server.out.log  644  3  102400  *  Z
${LOG_DIR}/server.err.log  644  3  102400  *  Z
EOF
    sudo chmod 0644 "$NEWSYSLOG_FILE"
  else
    echo "Could not configure newsyslog cap (sudo unavailable)." >&2
  fi
else
  echo "Could not configure newsyslog cap (sudo not found)." >&2
fi

echo "[4/6] Writing LaunchAgent plists..."
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
    <key>CIWI_AGENT_UPDATE_MANIFEST</key>
    <string>${MANIFEST_PATH}</string>
    <key>CIWI_AGENT_LAUNCHD_LABEL</key>
    <string>${LABEL}</string>
    <key>CIWI_AGENT_LAUNCHD_PLIST</key>
    <string>${PLIST_PATH}</string>
    <key>CIWI_AGENT_UPDATER_LABEL</key>
    <string>${UPDATER_LABEL}</string>
    <key>CIWI_AGENT_UPDATER_PLIST</key>
    <string>${UPDATER_PLIST_PATH}</string>
${GITHUB_TOKEN_ENV_BLOCK}
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

cat >"$UPDATER_PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${UPDATER_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/ciwi</string>
    <string>apply-staged-agent-update</string>
    <string>--manifest</string>
    <string>${MANIFEST_PATH}</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>CIWI_AGENT_UPDATE_MANIFEST</key>
    <string>${MANIFEST_PATH}</string>
    <key>CIWI_LAUNCHCTL_PATH</key>
    <string>/bin/launchctl</string>
    <key>PATH</key>
    <string>${INSTALL_DIR}:/usr/local/go/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>KeepAlive</key>
  <false/>
  <key>RunAtLoad</key>
  <false/>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/agent-updater.out.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/agent-updater.err.log</string>
</dict>
</plist>
EOF
chmod 0644 "$PLIST_PATH"
chmod 0644 "$UPDATER_PLIST_PATH"
chown "$(id -un)":staff "$PLIST_PATH" 2>/dev/null || true
chown "$(id -un)":staff "$UPDATER_PLIST_PATH" 2>/dev/null || true
plutil -lint "$PLIST_PATH" >/dev/null
plutil -lint "$UPDATER_PLIST_PATH" >/dev/null

echo "[5/6] Bootstrapping updater LaunchAgent..."
UID_NUM="$(id -u)"
launchctl bootout "gui/${UID_NUM}/${UPDATER_LABEL}" >/dev/null 2>&1 || true
launchctl bootout "gui/${UID_NUM}" "$UPDATER_PLIST_PATH" >/dev/null 2>&1 || true
launchctl disable "gui/${UID_NUM}/${UPDATER_LABEL}" >/dev/null 2>&1 || true
launchctl enable "gui/${UID_NUM}/${UPDATER_LABEL}" >/dev/null 2>&1 || true

if ! launchctl bootstrap "gui/${UID_NUM}" "$UPDATER_PLIST_PATH"; then
  echo "Updater LaunchAgent bootstrap failed. Diagnostics:" >&2
  launchctl print-disabled "gui/${UID_NUM}" | grep "${UPDATER_LABEL}" >&2 || true
  launchctl print "gui/${UID_NUM}/${UPDATER_LABEL}" >&2 || true
  log show --style syslog --last 5m --info --debug \
    --predicate "process == \"launchd\" AND composedMessage CONTAINS \"${UPDATER_LABEL}\"" 2>/dev/null | tail -n 50 >&2 || true
  exit 1
fi

echo "[6/6] Bootstrapping agent LaunchAgent..."
launchctl bootout "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true
launchctl bootout "gui/${UID_NUM}" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl disable "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true
launchctl enable "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true

if ! launchctl bootstrap "gui/${UID_NUM}" "$PLIST_PATH"; then
  echo "Agent LaunchAgent bootstrap failed. Diagnostics:" >&2
  launchctl print-disabled "gui/${UID_NUM}" | grep "${LABEL}" >&2 || true
  launchctl print "gui/${UID_NUM}/${LABEL}" >&2 || true
  log show --style syslog --last 5m --info --debug \
    --predicate "process == \"launchd\" AND composedMessage CONTAINS \"${LABEL}\"" 2>/dev/null | tail -n 50 >&2 || true
  exit 1
fi
launchctl kickstart -k "gui/${UID_NUM}/${LABEL}" >/dev/null 2>&1 || true

echo
echo "ciwi agent installed and started."
echo "Label:       ${LABEL}"
echo "Updater:     ${UPDATER_LABEL}"
echo "Binary:      ${INSTALL_DIR}/ciwi"
echo "Plist:       ${PLIST_PATH}"
echo "Updater plist: ${UPDATER_PLIST_PATH}"
echo "Server URL:  ${SERVER_URL}"
echo "Agent ID:    ${AGENT_ID}"
echo "Workdir:     ${WORKDIR}"
case "$TOKEN_SOURCE" in
  env) echo "GitHub token: set from CIWI_GITHUB_TOKEN (persisted in agent plist)" ;;
  existing-plist) echo "GitHub token: preserved from existing agent plist" ;;
  *) echo "GitHub token: not set (set CIWI_GITHUB_TOKEN before install to avoid API rate limits)" ;;
esac
echo "Logs:"
echo "  tail -f ${LOG_DIR}/agent.out.log ${LOG_DIR}/agent.err.log"
echo "Log cap:"
echo "  100MB via ${NEWSYSLOG_FILE} (agent + optional server logs)"
echo
echo "To uninstall:"
echo "  launchctl bootout gui/\$(id -u) ${PLIST_PATH} || true"
echo "  launchctl bootout gui/\$(id -u) ${UPDATER_PLIST_PATH} || true"
echo "  rm -f ${PLIST_PATH} ${UPDATER_PLIST_PATH} ${INSTALL_DIR}/ciwi"
