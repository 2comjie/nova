.PHONY: gen run

gen:
	diff-gen -dir ./item
	diff-gen -dir ./bag
	diff-gen -dir ./player

run: gen
	go run .
