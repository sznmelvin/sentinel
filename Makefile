.PHONY: build install test clean lint run

# Build the binary
build:
	go build -o sentinel .

# Install the binary to $GOPATH/bin
install:
	go install .

# Run tests
test:
	go test -v ./...

# Remove build artifacts
clean:
	rm -f sentinel
	rm -rf /dist/

# Run linting (requires golangci-lint)
lint:
	golangci-lint run ./...

# Run the application
run:
	go run .

# Tidy dependencies
tidy:
	go mod tidy

# Generate documentation
docs:
	@echo "Sentinel - TUI for Open Source Maintainers"
	@echo ""
	@echo "Usage:"
	@echo "  make build    - Build the binary"
	@echo "  make install  - Install to \$GOPATH/bin"
	@echo "  make run      - Run the application"
	@echo "  make test     - Run tests"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make tidy     - Tidy dependencies"
