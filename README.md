# gRPC Scan

A tool for discovering gRPC services and methods without needing protobuf files.

## Features

- **No Protobuf Files Needed** - Test gRPC services directly without .proto definitions
- **Direct Method Testing** - Call specific services/methods from the command line
- **Automatic Protocol Handling** - Just point at an endpoint and go
- **Zero Configuration** - Works out of the box with sensible defaults
- **Multi-Target Detection** - Scan multiple hosts to find gRPC services

**grpcurl needs proto files**
```
% grpcurl -vv -plaintext localhost:50051 ProductService.ListProducts
Error invoking method "ProductService.ListProducts": failed to query for service descriptor "ProductService": server does not support the reflection API
```

**while grpc-scan takes care of the protobuf tasks**
```
% grpc-scan -target localhost:50051 -wordlist data/grpc_common.txt   
[+] Scanning localhost:50051...
[+] Connected to gRPC service at localhost:50051
[+] Progress: 50/73 checked (8917/sec) | Found: 1 services[+] Found: proto.PingService
[+] Found: proto.UserService
[+] Found: proto.AuthService
[+] Found: proto.SecureService
[+] Found: proto.HelloService
[+] Found: proto.ProductService
[+] Completed: 73/73 checked | Found: 7 services      
```

## Installation

```bash
go build -o grpc-scan .
```

## Usage

### Basic Scanning

Discover services using reflection or smart bruteforce:
```bash
./grpc-scan -target=localhost:50051
```

### Direct Method Testing (No .proto files needed!)

Test a specific method directly:
```bash
# Call format: Service/Method
./grpc-scan -target=api.example.com:443 -call=UserService/GetUser
./grpc-scan -target=api.example.com:443 -call=proto.PingService/Ping
```

Test specific services and methods:
```bash
# Test specific service with default methods
./grpc-scan -target=api.example.com:443 -service=UserService

# Test multiple services
./grpc-scan -target=api.example.com:443 -service=UserService,AuthService

# Test specific methods on a service
./grpc-scan -target=api.example.com:443 -service=UserService -method=Login,Register,GetProfile
```

### Wordlist-Based Discovery

Use the comprehensive wordlist for thorough scanning:
```bash
./grpc-scan -target=api.example.com:443 -wordlist=data/grpc_wordlist.txt
```

Multi-threaded for faster scanning:
```bash
./grpc-scan -target=api.example.com:443 -wordlist=data/grpc_wordlist.txt -threads=50
```

### Multi-Target Detection

Detect gRPC services across multiple hosts:
```bash
# From file
./grpc-scan detect -targets=domains.txt -threads=100

# From stdin
cat targets.txt | ./grpc-scan detect -output=grpc_services.txt

# Output in JSON
./grpc-scan detect -targets=domains.txt -json -output=results.json
```

### Output Options

Save results to file:
```bash
./grpc-scan -target=api.example.com:443 -output=results.json
```

Simple output (service names only):
```bash
./grpc-scan -target=localhost:50051 -simple
```

Verbose mode for debugging:
```bash
./grpc-scan -target=localhost:50051 -v
```

## Comparison with grpcurl

Unlike grpcurl which requires protobuf files:
```bash
# grpcurl needs .proto files or server reflection
grpcurl -proto user.proto api.example.com:443 UserService/GetUser

# grpc-scan works without any proto files!
./grpc-scan -target=api.example.com:443 -call=UserService/GetUser
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
- `-call` - Call a specific service/method (format: Service/Method)
- `-service` - Test specific services (comma-separated)
- `-method` - Test specific methods (comma-separated)
- `-wordlist` - Path to wordlist file for service discovery
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
./grpc-scan -target=api.example.com:443 -wordlist=services.txt
```

With separate methods file:
```bash
./grpc-scan -target=api.example.com:443 -wordlist=services.txt -methods=methods.txt
```

Fast scanning with 50 threads:
```bash
./grpc-scan -target=api.example.com:443 -wordlist=enhanced.txt -threads=50
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

- **`grpc_wordlist.txt`** - Quick scan with common services

## License

MIT