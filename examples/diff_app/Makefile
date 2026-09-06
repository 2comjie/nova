.PHONY: gen proto run

TOOL_DIR := /tmp/nova-diff-proto-tools
PROTO_FILES := \
	diff_app/item/a_data_item.proto \
	diff_app/bag/a_data_bag.proto \
	diff_app/player/a_data_player.proto

gen:
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/item
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/bag
	go -C ../.. run ./cmd/diff-gen -dir ./examples/diff_app/player
	$(MAKE) proto

proto:
	mkdir -p $(TOOL_DIR) ./csharp
	GOWORK=off go -C ../.. build -o $(TOOL_DIR)/protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go
	cd .. && PATH=$(TOOL_DIR):$$PATH protoc -I . \
		--go_out=. \
		--go_opt=module=github.com/2comjie/nova/examples \
		$(PROTO_FILES)
	cd .. && protoc -I . --csharp_out=diff_app/csharp $(PROTO_FILES)

run: gen
	go run .
