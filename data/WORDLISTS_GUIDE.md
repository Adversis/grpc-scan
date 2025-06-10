# gRPC Scanner Wordlist Guide

## Overview

The gRPC scanner uses a single comprehensive wordlist (`grpc_wordlist.txt`) that combines:
- Common gRPC service names
- Real-world examples from grpc-go repository
- Industry-specific service patterns
- Standard gRPC services with proper namespaces
- Method name patterns

## Wordlist Format

The wordlist supports two formats:

### 1. Simple Service Names
```
UserService
auth
payment
```

### 2. Service with Methods
```
UserService:Login,Register,GetProfile
auth.AuthService:Authenticate,Validate
payment.v1.PaymentService:Process,Refund,GetStatus
```

### 3. Method Patterns (start with *)
```
*Get
*List
*Create
```

Method patterns starting with `*` are applied to all discovered services.

## Usage

```bash
# Use the wordlist for service discovery
./grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_wordlist.txt

# Combine with custom methods
./grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_wordlist.txt -methods=data/custom_methods.txt
```

## Wordlist Contents

The combined wordlist includes:
- **~600+ service patterns** covering common naming conventions
- **Authentication services** (auth, authentication, identity, oauth, etc.)
- **Business services** (user, product, order, payment, inventory, etc.)
- **Infrastructure services** (health, metrics, logging, monitoring, etc.)
- **Cloud-native patterns** (kubernetes, docker, cloud services)
- **Standard gRPC services** with proper namespacing
- **Method patterns** for comprehensive enumeration

## Creating Custom Wordlists

You can create your own wordlist following the format above. Tips:
1. Include both short names (e.g., `user`) and full patterns (e.g., `UserService`)
2. Add method mappings for services you know about
3. Use wildcards for common method patterns
4. Include versioned patterns (e.g., `v1.UserService`, `api.v2.UserService`)

## Performance Tips

- The scanner uses parallel threads (default: 10, configurable with `-threads`)
- Service names are tried with multiple patterns automatically
- Method discovery happens after service confirmation for efficiency