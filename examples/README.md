# gRPC Scanner Examples

This directory contains example code demonstrating various features of gRPC Scanner.

## auth_testing/

Demonstrates how gRPC Scanner can detect services that require authentication. This example shows:

- Testing services without authentication (should fail)
- Creating authentication tokens
- Using Bearer tokens for authentication
- Using API keys for authentication
- Services that don't require authentication

### Running the Authentication Example

First, start the demo server:
```bash
make run-server
```

Then run the authentication test:
```bash
go run examples/auth_testing/main.go
```

**Note**: The credentials in these examples (like "demo-api-key-123", "testuser", "testpass") are for demonstration purposes only and should never be used in production environments.