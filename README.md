# gRPC Scanner

A tool for scanning and analyzing gRPC endpoints.  The app should take a given URL/endpoint, and brute force RPC endpoints. If they don't exist the server typically returns a certain error (and the tool should assume that the endpoint does not exist). If the service or endpoint exist, a different response is returned (for example, the definition is incorrect), and the tool should say the endpoint is correct.


		fmt.Println("\nNote: A service is considered to exist if any RPC call returns an error that doesn't")
		fmt.Println("      explicitly state the service doesn't exist. This binary approach is designed to")
		fmt.Println("      detect services even when methods require specific parameters or authentication.")
		fmt.Println("      Use --verbose for detailed error information on each method call.")
    
## Project Structure

- `main.go` - The main gRPC scanner application
- `cmd/server/` - A test gRPC server implementation
- `cmd/client/` - A simple gRPC client
- `proto/` - Protocol buffer definitions
- `data/` - Configuration files for standard services and methods

## Prerequisites

- Go 1.19 or newer
- Protocol Buffer Compiler (protoc)
- Go plugins for Protocol Buffers:
  - `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`
  - `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`

## Getting Started

1. Clone the repository
2. Install dependencies:
   ```
   go mod tidy
   ```

## Building

Build all binaries:

```
make build
```

## Configuration Files

The scanner uses three configuration files by default:

1. `data/standard_services.txt` - List of standard gRPC service paths to check
2. `data/standard_methods.json` - JSON mapping of service paths to their method names
3. `data/common_methods.txt` - List of common method names to try against any service when fuzzing

### Standard Services File Format

The standard services file is a text file with one service path per line. Comments start with `#` and are ignored.

Example:
```
grpc.health.v1.Health                    # Health checking
grpc.reflection.v1alpha.ServerReflection # Server reflection
```

### Standard Methods File Format

The standard methods file is a JSON file mapping service paths to their method names.

Example:
```json
{
  "grpc.health.v1.Health": [
    "Check", 
    "Watch", 
    "List"
  ]
}
```

### Common Methods File Format

The common methods file is a text file with one method name per line, grouped by category. Comments start with `#` and are ignored.

Example:
```
# CRUD Operations
Get
List
Create
Update
Delete

# Authentication
Login
Logout
Validate
```

## Running the Applications

### Start the Test Server

```
make run-server
```

Optional arguments:
- `ARGS="--port 50052"` - Change the server port (default: 50051)
- `ARGS="--health=false"` - Disable health service
- `ARGS="--reflection=false"` - Disable reflection service

### Run the Client

```
make run-client
```

Optional arguments:
- `ARGS="--target localhost:50052"` - Change the target server (default: localhost:50051)

### Run the Scanner

```
make run-scanner
```

Optional arguments:
- `ARGS="--target localhost:50052"` - Change the target server (default: localhost:50051)
- `ARGS="--output results.json"` - Change the output file (default: scan_results.json)
- `ARGS="--timeout 10"` - Change request timeout in seconds (default: 5)
- `ARGS="--verbose"` - Enable verbose output
- `ARGS="--brute"` - Enable brute force mode to guess services and methods
- `ARGS="--services-file path/to/services.txt"` - Use custom services file
- `ARGS="--methods-file path/to/methods.json"` - Use custom methods file
- `ARGS="--common-methods-file path/to/methods.txt"` - Use custom common methods file 
- `ARGS="--wordlist path/to/wordlist.txt"` - Use custom wordlist for brute forcing
- `ARGS="--fuzz-versions"` - Enable fuzzing of different API versions (v1, v2, v3)
- `ARGS="--max-version 5"` - Set maximum API version to fuzz (default: 3)
- `ARGS="--fuzz-methods"` - Enable fuzzing common methods across all services
- `ARGS="--method-limit 20"` - Limit the number of methods to try per service when fuzzing (default: 0 = unlimited)

## Fuzzing Features

The scanner includes two powerful fuzzing features that can be used independently or together:

### Version Fuzzing

When enabled with `--fuzz-versions`, the scanner will:

1. Take each service with a version pattern (e.g., `.v1.`, `.v2.`) in its path
2. Generate variations with different version numbers up to the specified `--max-version`
3. Test each variation with appropriate methods

Example: If your services file contains `user.v1.UserService`, the version fuzzing will also test:
- `user.v2.UserService`
- `user.v3.UserService` (and so on up to max-version)

### Method Fuzzing

When enabled with `--fuzz-methods`, the scanner will:

1. Load a list of common method names from `data/common_methods.txt`
2. Try these common methods against all specified services
3. This allows discovering methods that might not be explicitly mapped to services

You can use `--method-limit` to control how many methods to try per service (to reduce scan time and traffic). When a limit is set, the scanner will prioritize the most common methods like Get, List, Create, etc.

For example:
```
make run-scanner ARGS="--target localhost:50051 --fuzz-methods --method-limit 20 --verbose"
```

### Combined Fuzzing

You can combine both fuzzing techniques for maximum coverage:
```
make run-scanner ARGS="--target localhost:50051 --fuzz-versions --fuzz-methods --verbose"
```

This will test many service versions with many methods, which can significantly increase the scan time but provides the most comprehensive results.

## Testing

To test the complete workflow:

1. Start the test server in one terminal:
   ```
   make run-server
   ```

2. Run the scanner in another terminal:
   ```
   make run-scanner
   ```

3. Examine the generated scan_results.json file.

## Customizing Services and Methods

To customize the services and methods checked by the scanner:

1. Edit `data/standard_services.txt` to add or remove service paths
2. Edit `data/standard_methods.json` to modify the methods for each service 
3. Edit `data/common_methods.txt` to modify the generic methods for fuzzing
4. Or provide custom files using the corresponding command-line options


# To test
https://www.postman.com/amanraj1608/rugrumble/request/gpa60c0/gettransactiondetails

`grpc://idx.parrot.bid:50051/blockchain.indexer.api.BlockchainService/GetTransactionDetails`

https://www.postman.com/weltcorp/dta-waud/grpc-request/646dc6567eb88434bf5535d9
`dta-waud-api-prod.weltcorp.com:443`