#!/bin/bash
set -e

script_dir="$(cd "$(dirname "$0")" && pwd)"
tool_dir="${TMPDIR:-/tmp}/nova-protoc-tools"
mkdir -p "$tool_dir"

GOWORK=off go -C "$script_dir" build -o "$tool_dir/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go
GOWORK=off go -C "$script_dir/cmd/protoc-gen-go-grpc-locator" build -o "$tool_dir/protoc-gen-go-grpc-locator" .

PATH="$tool_dir:$PATH" protoc \
  --proto_path="$script_dir/proto" \
  --go_out="$script_dir" \
  --go_opt=module=github.com/2comjie/nova \
  --go-grpc-locator_out="$script_dir" \
  --go-grpc-locator_opt=module=github.com/2comjie/nova \
  "$script_dir/proto/rpc/error.proto" \
  "$script_dir/proto/transport/actor/actor.proto" \
  "$script_dir/proto/transport/node/node.proto" \
  "$script_dir/proto/transport/gate/gate.proto"
