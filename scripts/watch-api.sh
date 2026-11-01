#!/usr/bin/env bash
set -u
bin=bin/sento-watch mark=bin/.sento-watch-built pid=
stop() { if [ -n "$pid" ]; then kill "$pid" 2>/dev/null; wait "$pid"; fi; }
trap 'stop; exit 0' INT TERM
mkdir -p bin && touch "$mark"
go build -o "$bin" ./cmd/sento || exit 1
"$bin" & pid=$!
while sleep 2; do
  if ! kill -0 "$pid" 2>/dev/null; then wait "$pid"; exit; fi
  changed=$(find cmd internal go.mod go.sum -newer "$mark" \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -print -quit)
  [ -n "$changed" ] || continue
  touch "$mark"
  echo "run:watch: $changed changed, rebuilding" >&2
  if go build -o "$bin.new" ./cmd/sento; then
    stop; mv "$bin.new" "$bin"; "$bin" & pid=$!
  else
    echo "BUILD FAILED: still serving the previous bin/sento-watch" >&2
  fi
done
