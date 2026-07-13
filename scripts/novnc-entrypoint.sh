#!/usr/bin/env bash
set -euo pipefail

export DISPLAY="${DISPLAY:-:99}"
DISPLAY_WIDTH="${DISPLAY_WIDTH:-1440}"
DISPLAY_HEIGHT="${DISPLAY_HEIGHT:-900}"
VNC_PORT="${VNC_PORT:-5900}"
NOVNC_PORT="${NOVNC_PORT:-6080}"
NOVNC_LISTEN="${NOVNC_LISTEN:-0.0.0.0}"

if [ -z "${VNC_PASSWORD:-}" ]; then
  echo "VNC_PASSWORD is required for interactive maintenance." >&2
  exit 1
fi

mkdir -p /tmp/kling-novnc "${DATA_DIR:-/data}"
rm -f /tmp/.X99-lock /tmp/.X11-unix/X99
Xvfb "$DISPLAY" -screen 0 "${DISPLAY_WIDTH}x${DISPLAY_HEIGHT}x24" -ac +extension RANDR &
XVFB_PID=$!
sleep 1
fluxbox >/tmp/kling-novnc/fluxbox.log 2>&1 &
FLUXBOX_PID=$!
x11vnc -storepasswd "$VNC_PASSWORD" /tmp/kling-novnc/vnc.pass >/dev/null
x11vnc -display "$DISPLAY" -forever -nevershared -localhost -nofilexfer -norepeat \
  -rfbport "$VNC_PORT" -rfbauth /tmp/kling-novnc/vnc.pass >/tmp/kling-novnc/x11vnc.log 2>&1 &
VNC_PID=$!
websockify --web=/usr/share/novnc "${NOVNC_LISTEN}:${NOVNC_PORT}" "localhost:$VNC_PORT" >/tmp/kling-novnc/websockify.log 2>&1 &
NOVNC_PID=$!

export BROWSER_HEADLESS=false
export CHROME_EXECUTABLE="${CHROME_EXECUTABLE:-/usr/bin/chromium}"
export NOVNC_URL="${NOVNC_URL:-http://127.0.0.1:${NOVNC_PORT}/vnc.html?autoconnect=1&resize=scale}"

cleanup() {
  kill "${APP_PID:-}" "$NOVNC_PID" "$VNC_PID" "$FLUXBOX_PID" "$XVFB_PID" 2>/dev/null || true
  rm -f /tmp/.X99-lock /tmp/.X11-unix/X99 /tmp/kling-novnc/vnc.pass
}
trap cleanup EXIT TERM INT

"$@" &
APP_PID=$!
wait -n "$APP_PID" "$NOVNC_PID" "$VNC_PID" "$FLUXBOX_PID" "$XVFB_PID"
