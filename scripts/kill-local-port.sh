#!/usr/bin/env sh
set -eu

usage() {
  echo "Usage: $0 <port|localhost-url>"
  echo "Examples:"
  echo "  $0 59629"
  echo "  $0 :59629"
  echo "  $0 127.0.0.1:59629"
  echo "  $0 http://127.0.0.1:59629"
}

if [ "$#" -ne 1 ]; then
  usage >&2
  exit 2
fi

input=$1
target=${input#http://}
target=${target#https://}
target=${target%%/*}
target=${target%%\?*}
target=${target%%#*}
port=${target##*:}

case "$port" in
  ''|*[!0-9]*)
    echo "Invalid port or localhost URL: $input" >&2
    usage >&2
    exit 2
    ;;
esac

if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then
  echo "Port out of range: $port" >&2
  exit 2
fi

pids=$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)

if [ -z "$pids" ]; then
  echo "No process is listening on port $port"
  exit 0
fi

echo "$pids" | xargs kill
echo "Killed process(es) on port $port: $(echo "$pids" | tr '\n' ' ')"
