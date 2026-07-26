#!/bin/bash
protoc \
  --proto_path=proto \
  --go_out=. \
  --go_opt=module=github.com/2comjie/wali \
  --go-grpc-locator_out=. \
  --go-grpc-locator_opt=module=github.com/2comjie/wali \
  proto/transport/node/node.proto \
  proto/transport/gate/gate.proto
