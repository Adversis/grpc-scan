# Build stage
FROM golang:1.21-alpine AS builder

# Install dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o grpc-scan .

# Final stage
FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 -S grpcscan && \
    adduser -u 1000 -S grpcscan -G grpcscan

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/grpc-scan /app/grpc-scan

# Copy data files
COPY --from=builder /app/data /app/data

# Change ownership
RUN chown -R grpcscan:grpcscan /app

# Switch to non-root user
USER grpcscan

# Set entrypoint
ENTRYPOINT ["/app/grpc-scan"]