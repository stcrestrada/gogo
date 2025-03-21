.PHONY: all fmt test build clean examples check

all: fmt test build

# Format code
fmt:
	go fmt ./...

# Run tests
test:
	go test -v ./...

# Build library
build:
	go build -v ./...

# Build examples
examples:
	go build -o examples_bin ./examples/examples.go

# Run examples
run-examples: examples
	./examples_bin

# Run a more comprehensive check (format + verify + test)
check:
	go fmt ./...
	go mod verify
	go vet ./...
	go test -v ./...

# Clean build artifacts
clean:
	rm -f examples_bin