#!/bin/bash
cd "$(dirname "$0")"
protoc \
  --proto_path=proto \
  --go_out=.. \
  --go_opt=module=github.com/2comjie/wali/examples \
  --go-grpc_out=.. \
  --go-grpc_opt=module=github.com/2comjie/wali/examples \
  demo.proto
echo "generated pbDemo/demo.pb.go pbDemo/demo_grpc.pb.go"
