# gRPC Scanner

A simple, powerful tool for discovering gRPC services and methods on any endpoint.

## Features

- **Automatic Protocol Handling** - Just point at an endpoint and go
- **Smart Discovery** - Uses reflection when available, falls back to intelligent pattern matching
- **Zero Configuration** - Works out of the box with sensible defaults
- **Fast & Concurrent** - Parallel service checking for quick results

## Installation

```bash
go build -o grpc-scanner .
```

## Usage

Basic scan:
```bash
./grpc-scanner -target=localhost:50051
```

Wordlist-based brute force:
```bash
./grpc-scanner -target=api.example.com:443 -wordlist=wordlist.txt
```

Multi-threaded wordlist scan:
```bash
./grpc-scanner -target=api.example.com:443 -wordlist=services.txt -threads=50 -v
```

Save results to file:
```bash
./grpc-scanner -target=api.example.com:443 -output=results.json
```

Verbose mode for debugging:
```bash
./grpc-scanner -target=localhost:50051 -v
```

Simple output (just service names):
```bash
./grpc-scanner -target=localhost:50051 -simple
```

## How It Works

1. **Connects** to the gRPC endpoint
2. **Tries reflection** first (the most accurate discovery method)
3. **Checks standard services** (health, reflection, etc.)
4. **Smart pattern matching** if reflection isn't available:
   - Tests common service naming patterns
   - Identifies services based on error responses
   - Discovers methods for each found service

## Options

- `-target` - gRPC server address (default: localhost:50051)
- `-wordlist` - Path to wordlist file for service brute forcing
- `-threads` - Number of concurrent threads (default: 10)
- `-timeout` - Timeout in seconds (default: 10)
- `-output` - Save results to JSON file (default: stdout)
- `-v` - Verbose output for debugging
- `-simple` - Output just service names

## Output Example

```
=== gRPC Service Scanner Results ===
Target: localhost:50051
Mode: reflection
Services found: 3

• helloworld.Greeter
  - SayHello
• grpc.health.v1.Health
  - Check
• grpc.reflection.v1alpha.ServerReflection
  - ServerReflectionInfo

✓ Server reflection is enabled
```

## Wordlist Brute Forcing

The scanner supports advanced wordlist-based brute forcing with multiple formats:

### Basic Wordlist
Simple service names, one per line:
```
User
Auth
Product
```

### Enhanced Wordlist Format
Specify custom methods for each service:
```
# Service with specific methods
UserService:GetUser,CreateUser,Login,Logout
PaymentService:ProcessPayment,RefundPayment,GetStatus

# Global methods (prefix with *)
*GetById
*SearchByQuery
*BulkCreate
```

### Usage Examples

Basic wordlist scan:
```bash
./grpc-scanner -target=api.example.com:443 -wordlist=services.txt
```

With separate methods file:
```bash
./grpc-scanner -target=api.example.com:443 -wordlist=services.txt -methods=methods.txt
```

Fast scanning with 50 threads:
```bash
./grpc-scanner -target=api.example.com:443 -wordlist=enhanced.txt -threads=50
```

### Pattern Generation
For each service in the wordlist, the scanner automatically tries:
- Raw name: `User`
- Service suffix: `UserService`
- Package pattern: `user.UserService`
- API pattern: `api.User`
- Versioned: `user.v1.UserService`

### Included Wordlists

The `data/` directory contains several optimized wordlists:

- **`grpc_common.txt`** - Quick scan with ~100 most common services
- **`grpc_comprehensive.txt`** - Thorough scan with 500+ services
- **`grpc_examples_based.txt`** - Services from grpc-go examples
- **`methods_comprehensive.txt`** - 300+ method names

See [data/WORDLISTS_GUIDE.md](data/WORDLISTS_GUIDE.md) for detailed usage.

Example:
```bash
# Quick scan
./grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_common.txt

# Comprehensive scan
./grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_comprehensive.txt -threads=50
```

### Generating Wordlists from API Documentation

The scanner includes built-in wordlist generation from API documentation:

```bash
# Extract from URL
./grpc-scanner wordlist -url=https://dev.evernote.com/doc/reference/ -output=evernote_services.txt

# Extract from local file
./grpc-scanner wordlist -input=api_docs.html -output=services.txt

# Use the generated wordlist
./grpc-scanner -target=api.evernote.com:443 -wordlist=evernote_services.txt
```

See [README_API2WORDLIST.md](README_API2WORDLIST.md) for detailed usage.

## Smart Pattern Matching

When reflection is disabled and no wordlist is provided, the scanner uses intelligent patterns:

- Common service structures: `Service`, `ServiceName`, `package.Service`
- Versioned services: `service.v1.ServiceName`
- Business domains: User, Auth, Product, Order, Payment, etc.
- Method patterns: Get, List, Create, Update, Delete, plus domain-specific methods

The scanner distinguishes between "service not found" and other errors (auth, parameters, etc.) to accurately identify existing services.

## License

MIT