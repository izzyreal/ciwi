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
  if command -v getent >/dev/null 2>&1; then
    name="$(getent hosts "$ip" 2>/dev/null | awk 'NF>=2 {print $2; exit}' | sed 's/\.$//' || true)"
    if [ -n "$name" ]; then
      printf '%s\n' "$name"
      return
    fi
  fi
  if command -v dig >/dev/null 2>&1; then
    name="$(dig +short -x "$ip" 2>/dev/null | sed 's/\.$//' | sed -n '1p' || true)"
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

discover_search_domains() {
  out=""
  if [ -f /etc/resolv.conf ]; then
    domains="$(awk '
      /^[[:space:]]*(search|domain)[[:space:]]+/ {
        for (i = 2; i <= NF; i++) print tolower($i)
      }
    ' /etc/resolv.conf | sed 's/\.$//' | sed '/^$/d' | sort -u || true)"
    if [ -n "$domains" ]; then
      out="$domains"
    fi
  fi
  if [ -z "$out" ] && command -v hostname >/dev/null 2>&1; then
    d="$(hostname -d 2>/dev/null | tr '[:upper:]' '[:lower:]' | sed 's/\.$//' || true)"
    if [ -n "$d" ]; then
      out="$d"
    fi
  fi
  if [ -z "$out" ]; then
    out="local lan"
  fi
  printf '%s\n' "$out" | sed '/^$/d' | sort -u
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
    browse_tmp="$(mktemp)"
    dns-sd -B _ciwi._tcp local >"$browse_tmp" 2>/dev/null &
    browse_pid=$!
    sleep 2
    kill "$browse_pid" >/dev/null 2>&1 || true
    wait "$browse_pid" >/dev/null 2>&1 || true
    for name in $(awk '/Add/ {print $NF}' "$browse_tmp" | sort -u); do
      resolve_tmp="$(mktemp)"
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

  if command -v avahi-browse >/dev/null 2>&1; then
    while IFS= read -r endpoint; do
      [ -n "$endpoint" ] || continue
      if probe_server "$endpoint"; then
        found="$(append_unique "$found" "$endpoint")"
      fi
    done <<EOF
$(avahi-browse -rt _ciwi._tcp 2>/dev/null | awk -F';' '/^=/ && NF >= 9 { host=$7; port=$9; sub(/\.$/, "", host); if (host != "" && port != "") print "http://" tolower(host) ":" port }' | sort -u)
EOF
  fi

  if command -v dig >/dev/null 2>&1; then
    for domain in $(discover_search_domains); do
      [ -n "$domain" ] || continue
      for ptr in $(dig +short "PTR" "_ciwi._tcp.${domain}" 2>/dev/null | sed 's/\.$//' | sort -u); do
        [ -n "$ptr" ] || continue
        srv_lines="$(dig +short "SRV" "$ptr" 2>/dev/null || true)"
        while read -r p1 p2 port host; do
          host="$(printf '%s' "${host:-}" | sed 's/\.$//' | tr '[:upper:]' '[:lower:]')"
          port="$(printf '%s' "${port:-}" | tr -d '[:space:]')"
          [ -n "$host" ] && [ -n "$port" ] || continue
          endpoint="http://${host}:${port}"
          if probe_server "$endpoint"; then
            found="$(append_unique "$found" "$endpoint")"
          fi
        done <<EOF
$srv_lines
EOF
      done
    done
  fi

  if [ -f /etc/hosts ]; then
    for host in $(awk '
      /^[[:space:]]*#/ { next }
      NF >= 2 {
        for (i = 2; i <= NF; i++) {
          h=tolower($i)
          sub(/\.$/, "", h)
          if (h == "localhost") continue
          if (h ~ /localhost$/) continue
          print h
        }
      }
    ' /etc/hosts | sort -u); do
      endpoint="http://${host}:8112"
      if probe_server "$endpoint"; then
        found="$(append_unique "$found" "$endpoint")"
      fi
    done
  fi

  if command -v ip >/dev/null 2>&1; then
    for ip in $(ip neigh 2>/dev/null | awk '{print $1}' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -u); do
      endpoint="http://${ip}:8112"
      if probe_server "$endpoint"; then
        found="$(append_unique "$found" "$endpoint")"
        continue
      fi
      host="$(resolve_hostname_for_ip "$ip")"
      if [ -n "$host" ]; then
        endpoint="http://${host}:8112"
        if probe_server "$endpoint"; then
          found="$(append_unique "$found" "$endpoint")"
        fi
      fi
    done
  fi

  if command -v arp >/dev/null 2>&1; then
    for ip in $(arp -an 2>/dev/null | awk '{print $2}' | tr -d '()' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -u); do
      endpoint="http://${ip}:8112"
      if probe_server "$endpoint"; then
        found="$(append_unique "$found" "$endpoint")"
        continue
      fi
      host="$(resolve_hostname_for_ip "$ip")"
      if [ -n "$host" ]; then
        endpoint="http://${host}:8112"
        if probe_server "$endpoint"; then
          found="$(append_unique "$found" "$endpoint")"
        fi
      fi
    done
  fi

  if command -v nc >/dev/null 2>&1 && command -v ip >/dev/null 2>&1; then
    prefixes="$( (ip -4 addr show scope global 2>/dev/null | awk '/inet /{split($2,a,"/"); split(a[1],o,"."); if (length(o)==4) print o[1]"."o[2]"."o[3] }'; printf '%s\n' '192.168.1' '192.168.0' '10.0.0' '10.0.1' '172.16.0' '172.20.0') | sed '/^$/d' | sort -u)"
    probes=0
    for prefix in $prefixes; do
      for n in $(seq 1 254); do
        probes=$((probes + 1))
        if [ "$probes" -gt 768 ]; then
          break 2
        fi
        ip="${prefix}.${n}"
        if nc -z -w 1 "$ip" 8112 >/dev/null 2>&1; then
          endpoint="http://${ip}:8112"
          if probe_server "$endpoint"; then
            found="$(append_unique "$found" "$endpoint")"
            break 2
          fi
        fi
      done
    done
  fi

  printf '%s\n' "$found" | sed '/^$/d'
}

choose_server_url() {
  discovered="$(discover_servers)"
  count="$(printf '%s\n' "$discovered" | sed '/^$/d' | wc -l | tr -d ' ')"

  if [ "$count" -eq 1 ]; then
    SERVER_URL_SOURCE="auto-discovery (single match)"
    printf '%s\n' "$discovered" | sed -n '1p'
    return
  fi

  if [ "$count" -gt 1 ]; then
    SERVER_URL_SOURCE="auto-discovery (user selected)"
    echo "Multiple ciwi servers discovered:" >&2
    i=0
    printf '%s\n' "$discovered" | while IFS= read -r url; do
      i=$((i + 1))
      echo "  [$i] $url" >&2
    done
    printf "Choose server number [1]: " >&2
    if [ -t 0 ]; then
      read -r choice
    else
      choice="1"
    fi
    if [ -z "${choice:-}" ]; then
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

  if [ ! -t 0 ]; then
    SERVER_URL_SOURCE="default fallback (non-interactive)"
    printf '%s\n' "http://127.0.0.1:8112"
    return
  fi

  printf "No ciwi server auto-discovered. Enter server URL (example http://bhakti.local:8112): " >&2
  read -r entered
  entered="$(printf '%s' "$entered" | tr -d '[:space:]')"
  if [ -z "$entered" ]; then
    echo "server URL is required" >&2
    exit 1
  fi
  SERVER_URL_SOURCE="manual entry (no server auto-discovered)"
  entered="$(canonicalize_url "$entered")"
  printf '%s\n' "$(prefer_hostname_url "$entered")"
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

SERVER_URL=""
SERVER_URL_SOURCE=""
if [ -n "${CIWI_SERVER_URL:-}" ]; then
  SERVER_URL="$(canonicalize_url "${CIWI_SERVER_URL}")"
  SERVER_URL_SOURCE="CIWI_SERVER_URL environment variable"
else
  SERVER_URL="$(choose_server_url)"
fi
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
echo "[info] Configuring CIWI_SERVER_URL=${SERVER_URL} (source: ${SERVER_URL_SOURCE})"
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
