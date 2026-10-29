#!/usr/bin/env bash
# Compila o frontend (embutido no binário) e o backend em bin/netguardd.
#   scripts/compilar.sh            # para este PC
#   GOARCH=arm64 scripts/compilar.sh   # para o Raspberry Pi (64 bits)
set -euo pipefail
cd "$(dirname "$0")/.."

versao=$(git describe --tags --always --dirty 2>/dev/null || echo dev)

echo "==> frontend"
(cd frontend && npm ci --no-audit --no-fund >/dev/null && npm run build)

echo "==> backend ($versao, ${GOARCH:-$(go env GOARCH)})"
mkdir -p bin
(cd backend && CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X netguard/internal/api.Versao=$versao" \
  -o ../bin/netguardd ./cmd/netguardd)

ls -lh bin/netguardd
