#!/usr/bin/env sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

if ! command -v go >/dev/null 2>&1; then
	echo "go is required but was not found in PATH" >&2
	exit 1
fi

cd "$REPO_ROOT"

echo "Installing proto-gen-rpc-wali..."
go install ./cmd/proto-gen-rpc-wali

if [ -n "${GOBIN:-}" ]; then
	BIN_DIR=$GOBIN
else
	BIN_DIR=$(go env GOPATH)/bin
fi

BIN_PATH="$BIN_DIR/proto-gen-rpc-wali"

echo "Installed: $BIN_PATH"
case ":$PATH:" in
	*":$BIN_DIR:"*) ;;
	*)
		echo "Warning: $BIN_DIR is not in PATH" >&2
		echo "Add it with: export PATH=\"\$PATH:$BIN_DIR\"" >&2
		;;
esac
