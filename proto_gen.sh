#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

MODULE="github.com/2comjie/wali"
PROTOS=$(find . -name '*.proto')

# --go_out        生成 message 结构体 (protoc-gen-go)
# --go-wali-grpc_out  生成 service 桩代码 (proto-gen-rpc-wali，内含 grpc 生成)
protoc -I . -I ./proto \
  --go_out=. --go_opt=module="$MODULE" \
  --plugin=protoc-gen-go-wali-grpc="$(go env GOPATH)/bin/proto-gen-rpc-wali" \
  --go-wali-grpc_out=. --go-wali-grpc_opt=module="$MODULE" \
  $PROTOS
