.PHONY: gen proto run

PROTO_FILES := \
	item/a_data_item.proto \
	bag/a_data_bag.proto \
	player/a_data_player.proto

gen:
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/item -proto-dir ./examples/proto
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/bag -proto-dir ./examples/proto
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/player -proto-dir ./examples/proto
	$(MAKE) proto

proto:
	@set -e; tools_dir=$$(mktemp -d); \
	GOWORK=off go -C ../.. build -o "$$tools_dir/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go; \
	protoc -I ../proto --plugin=protoc-gen-go="$$tools_dir/protoc-gen-go" \
		--go_out=.. --go_opt=module=github.com/2comjie/nova/examples \
		--csharp_out=./csharp $(PROTO_FILES)

run: gen
	go run .
