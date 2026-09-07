.PHONY: gen run test

gen:
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/profile -proto-dir ./examples/proto
	@set -e; tools_dir=$$(mktemp -d); \
	GOWORK=off go -C ../.. build -o "$$tools_dir/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go; \
	protoc -I ../proto --plugin=protoc-gen-go="$$tools_dir/protoc-gen-go" \
		--go_out=.. --go_opt=module=github.com/2comjie/nova/examples profile/profile.proto

run:
	go run .

test:
	go test ./...
