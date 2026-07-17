.PHONY: build run test lint fmt install clean run-staging snapshot

# Build the binary
build:
	go build -o bin/inc .

# Build and run with args: make run ARGS="incidents list"
run: build
	./bin/inc $(ARGS)

# Run all tests
test:
	go test ./...

# Lint (golangci-lint)
lint:
	golangci-lint run

# Format
fmt:
	go fmt ./...
	goimports -local github.com/incident-io/inc -w .

# Install to GOPATH/bin
install:
	go install .

# Clean build artifacts
clean:
	rm -rf bin/ dist/ completions/

# Run against staging
run-staging: build
	INCIDENT_API_URL=https://api.staging.incident.io ./bin/inc $(ARGS)

# Snapshot release (test goreleaser locally without publishing)
snapshot:
	goreleaser release --snapshot --clean
