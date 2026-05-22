#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

MODULE="github.com/2comjie/wali"
PROTOS=$(find . -name '*.proto')

protoc -I . -I ./proto \
  --go_out=. --go_opt=module="$MODULE" \
  --go-grpc_out=. --go-grpc_opt=module="$MODULE" \
  $PROTOS
