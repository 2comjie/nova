#!/bin/bash
set -e

cd "$(dirname "$0")"

PROTO_DIR="./proto"
OUT_DIR="../internal/pb"



run_protoc() {
    local output exit_code
    output=$(protoc "$@" 2>&1)
    exit_code=$?
    [ -n "$output" ] && echo "$output" | grep -v -E "(warning:|Warning:)" || true
    return $exit_code
}

gen_rpc() {
    (
    cd "$PROTO_DIR"
    shopt -s nullglob
    local files=()
    local pattern matches
    for pattern in "$@"; do
        matches=( $pattern )
        files+=("${matches[@]}")
    done
    [ "${#files[@]}" -eq 0 ] && return 0
    run_protoc \
        -I. \
        --go_out="$OUT_DIR" \
        --go_opt=paths=source_relative \
        --wali-grpc_out="$OUT_DIR" \
        --wali-grpc_opt=paths=source_relative \
        "${files[@]}"
    )
}

gen_pb() {
    (
    cd "$PROTO_DIR"
    shopt -s nullglob
    local files=()
    local pattern matches
    for pattern in "$@"; do
        matches=( $pattern )
        files+=("${matches[@]}")
    done
    [ "${#files[@]}" -eq 0 ] && return 0
    run_protoc \
        -I. \
        --go_out="$OUT_DIR" \
        --go_opt=paths=source_relative \
        "${files[@]}"
    )
}


gen_rpc transport/node/*.proto
gen_rpc transport/gate/*.proto
gen_pb transport/request/*.proto
