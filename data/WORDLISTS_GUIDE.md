# gRPC Scanner Wordlists Guide

This directory contains several wordlists optimized for gRPC service discovery. Each wordlist serves a different purpose and scanning strategy.

## Available Wordlists

### 1. `grpc_common.txt` - Quick Scan (Recommended for First Pass)
- **Size**: ~100 entries
- **Purpose**: Fast discovery of most common gRPC services
- **Use case**: Initial reconnaissance, time-sensitive scans
- **Content**: Most frequently seen services and methods based on real-world usage

```bash
./grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_common.txt -threads=20
```

### 2. `grpc_comprehensive.txt` - Thorough Scan
- **Size**: ~500+ entries
- **Purpose**: Comprehensive service discovery
- **Use case**: Detailed security assessments, finding hidden services
- **Content**: Extensive list including industry-specific services

```bash
./grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_comprehensive.txt -threads=50
```

### 3. `grpc_examples_based.txt` - Example-Based Scan
- **Size**: ~150 entries
- **Purpose**: Find services based on common gRPC examples and tutorials
- **Use case**: Development/staging environments, services based on examples
- **Content**: Actual services from grpc-go examples repository

```bash
./grpc-scanner -target=dev.example.com:50051 -wordlist=data/grpc_examples_based.txt
```

### 4. `methods_comprehensive.txt` - Methods Only
- **Size**: ~300+ methods
- **Purpose**: Thorough method discovery when services are known
- **Use case**: Second-pass scanning, method enumeration
- **Content**: Comprehensive list of gRPC method names

```bash
# Use with discovered services
./grpc-scanner -target=api.example.com:443 -wordlist=services.txt -methods=data/methods_comprehensive.txt
```

## Wordlist Formats

### Enhanced Format (Service:Methods)
```
UserService:GetUser,CreateUser,UpdateUser,DeleteUser
AuthService:Login,Logout,Authenticate,ValidateToken
```

### Simple Format (Service Names Only)
```
UserService
AuthService
ProductService
```

### Global Methods (Prefix with *)
```
*Get
*List
*Create
*Update
*Delete
```

## Scanning Strategies

### 1. Quick Discovery
Start with the common wordlist for fast results:
```bash
./grpc-scanner -target=target.com:443 -wordlist=data/grpc_common.txt -threads=20
```

### 2. Comprehensive Scan
Use the comprehensive wordlist with more threads:
```bash
./grpc-scanner -target=target.com:443 -wordlist=data/grpc_comprehensive.txt -threads=50 -v
```

### 3. Two-Phase Approach
First discover services, then enumerate methods:
```bash
# Phase 1: Discover services
./grpc-scanner -target=target.com:443 -wordlist=data/grpc_common.txt -simple > found_services.txt

# Phase 2: Enumerate methods on found services
./grpc-scanner -target=target.com:443 -wordlist=found_services.txt -methods=data/methods_comprehensive.txt
```

### 4. Custom Wordlist from API Docs
Generate a targeted wordlist from API documentation:
```bash
# Extract from documentation
./api2wordlist -url=https://api.target.com/docs -output=custom_wordlist.txt

# Use the generated wordlist
./grpc-scanner -target=api.target.com:443 -wordlist=custom_wordlist.txt
```

## Tips for Effective Scanning

1. **Start Small**: Begin with `grpc_common.txt` to quickly identify if the target has gRPC services

2. **Increase Threads Carefully**: More threads = faster scanning, but may trigger rate limits
   - Development servers: 10-20 threads
   - Production APIs: 5-10 threads
   - Local testing: 50+ threads

3. **Use Verbose Mode**: Add `-v` flag to see what's being tested and understand the results better

4. **Combine Wordlists**: For thorough testing, combine multiple wordlists:
   ```bash
   cat data/grpc_common.txt data/grpc_examples_based.txt | sort -u > combined.txt
   ./grpc-scanner -target=target.com:443 -wordlist=combined.txt
   ```

5. **Service-Specific Testing**: If you know the industry/domain, extract relevant sections:
   ```bash
   # Extract e-commerce services
   grep -A5 "E-commerce" data/grpc_comprehensive.txt > ecommerce_services.txt
   ```

## Creating Custom Wordlists

### From Protobuf Files
If you have access to .proto files:
```bash
# Extract service names
grep "service " *.proto | awk '{print $2}' > services.txt

# Extract method names
grep "rpc " *.proto | awk '{print $2}' | sed 's/(//g' > methods.txt
```

### From Application Code
Look for gRPC client calls:
```bash
# Go code
grep -r "pb\." --include="*.go" | grep "Client" | awk -F'.' '{print $2}' | sort -u

# Python code
grep -r "_pb2" --include="*.py" | grep "Stub" | awk -F'(' '{print $1}' | awk '{print $NF}'
```

### Industry-Specific Wordlists
Create focused wordlists for specific industries:
- **FinTech**: Payment, Transaction, Account, Ledger, Wallet
- **Healthcare**: Patient, Appointment, Prescription, Medical, Record
- **E-commerce**: Product, Order, Cart, Inventory, Shipping
- **SaaS**: Subscription, Tenant, License, Usage, Billing

## Security Considerations

1. **Rate Limiting**: Be respectful of target servers
   - Add delays between requests if needed
   - Monitor for 429 (Too Many Requests) errors

2. **Authorization**: Some services may require authentication
   - Methods may exist but return permission errors
   - This still confirms the service/method exists

3. **Logging**: Your scans may be logged by the target
   - Use only on authorized targets
   - Consider the noise you're generating

## Wordlist Maintenance

These wordlists are based on:
- Common gRPC patterns and conventions
- Real services from grpc-go examples
- Industry-standard naming conventions
- Popular gRPC frameworks and libraries

To contribute or suggest additions:
1. Identify new patterns from real gRPC services
2. Extract service/method names from public repositories
3. Add industry-specific terminology
4. Include new gRPC framework patterns