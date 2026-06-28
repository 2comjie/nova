#!/bin/bash
protoc \
    --proto_path=proto \
    --go_out=. \
    --go_opt=module=github.com/2comjie/wali \
    --go-grpc_out=. \
    --go-grpc_opt=module=github.com/2comjie/wali \
    proto/transport/request/request.proto \
    proto/transport/gate/gate.proto \
    proto/transport/node/node.proto