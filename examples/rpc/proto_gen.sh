#!/bin/bash
set -e

cd "$(dirname "$0")"

OUT_DIR="./pb"

mkdir -p "$OUT_DIR"

# 忽略 protoc 未使用警告的函数
run_protoc() {
    local output exit_code
    output=$(protoc "$@" 2>&1)
    exit_code=$?
    # 过滤警告信息，但保留错误信息和其他输出
    [ -n "$output" ] && echo "$output" | grep -v -E "(warning:|Warning:)" || true
    return $exit_code
}

# 生成普通 pb + wali grpc pb。
gen_rpc() {
    run_protoc \
        -I. \
        --go_out="$OUT_DIR" \
        --go_opt=paths=source_relative \
        --wali-grpc_out="$OUT_DIR" \
        --wali-grpc_opt=paths=source_relative \
        "$@"
}

# 只生成普通 pb，适合没有 service 的消息定义。
gen_pb() {
    run_protoc \
        -I. \
        --go_out="$OUT_DIR" \
        --go_opt=paths=source_relative \
        "$@"
}

# 后面新增 proto 目录，就照这个格式往下加。
gen_rpc proto/chat/*.proto
