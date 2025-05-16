 .PHONY: build run-server run-client run-scanner clean

# Build all binaries
build:
	@echo "Building applications..."
	@mkdir -p bin
	@go build -o bin/server cmd/server/main.go
	@go build -o bin/client cmd/client/main.go
	@go build -o bin/scanner main.go
	@echo "Build complete."

# Run the server
run-server:
	@echo "Starting the gRPC server..."
	@go run cmd/server/main.go $(ARGS)

# Run the client
run-client:
	@echo "Running gRPC client..."
	@go run cmd/client/main.go $(ARGS)

# Run the scanner
run-scanner:
	@echo "Running gRPC scanner..."
	@go run main.go $(ARGS)

# Clean up
clean:
	@echo "Cleaning up..."
	@rm -rf bin/
	@echo "Cleanup complete."

# Generate proto files
proto:
	@echo "Generating proto files..."
	@protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/service.proto
	@echo "Proto generation complete." 