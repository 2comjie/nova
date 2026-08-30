.PHONY: gen proto run

gen:
	diff-gen -dir ./item
	diff-gen -dir ./bag
	diff-gen -dir ./player
	$(MAKE) proto

proto:
	mkdir -p ./csharp
	cd .. && protoc -I . --csharp_out=diff_app/csharp \
		diff_app/item/a_data_item.proto \
		diff_app/item/a_data_item_diff.proto \
		diff_app/bag/a_data_bag.proto \
		diff_app/bag/a_data_bag_diff.proto \
		diff_app/player/a_data_player.proto \
		diff_app/player/a_data_player_diff.proto

run: gen
	go run .
