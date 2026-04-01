.PHONY: build run clean
build:
	CGO_ENABLED=0 go build -o saddlebag ./cmd/saddlebag/
run: build
	./saddlebag
clean:
	rm -f saddlebag
