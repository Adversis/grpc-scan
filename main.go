package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
)

// Default file paths
const (
	DefaultServicesFile      = "data/standard_services.txt"
	DefaultMethodsFile       = "data/standard_methods.json"
	DefaultCommonMethodsFile = "data/common_methods.txt"
)

// loadServicePathsFromFile reads gRPC service paths from a text file, skipping comments and empty lines
func loadServicePathsFromFile(filePath string) ([]string, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("service paths file not found: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open service paths file: %v", err)
	}
	defer file.Close()

	var services []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// If line has a comment, remove it
		if idx := strings.Index(line, "#"); idx > 0 {
			line = strings.TrimSpace(line[:idx])
		}

		services = append(services, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading service paths file: %v", err)
	}

	return services, nil
}

// loadMethodsFromFile reads gRPC methods from a JSON file
func loadMethodsFromFile(filePath string) (map[string][]string, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("methods file not found: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open methods file: %v", err)
	}
	defer file.Close()

	var methods map[string][]string
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&methods); err != nil {
		return nil, fmt.Errorf("failed to decode methods JSON: %v", err)
	}

	return methods, nil
}

// loadCommonMethodsFromFile reads common method names from a text file, skipping comments and empty lines
func loadCommonMethodsFromFile(filePath string) ([]string, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("common methods file not found: %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open common methods file: %v", err)
	}
	defer file.Close()

	var methods []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		methods = append(methods, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading common methods file: %v", err)
	}

	return methods, nil
}

// fallbackStandardServicePaths returns the default hardcoded service paths
func fallbackStandardServicePaths() []string {
	return []string{
		"grpc.health.v1.Health",                    // Health checking
		"grpc.reflection.v1alpha.ServerReflection", // Server reflection
		"grpc.reflection.v1.ServerReflection",      // Server reflection v1
		"grpc.channelz.v1.Channelz",                // Channel info service
		"grpc.status.v1.Status",                    // Status service
		"grpc.admin.v1.Admin",                      // Admin service
		"grpc.instrumentation.v1.Instrumentation",  // Instrumentation
		"grpc.lb.v1.LoadBalancer",                  // Load balancer
		"grpc.service_config.ServiceConfig",        // Service config
	}
}

// fallbackStandardMethods returns the default hardcoded methods
func fallbackStandardMethods() map[string][]string {
	return map[string][]string{
		"grpc.health.v1.Health": {
			"Check", "Watch", "List",
		},
		"grpc.reflection.v1alpha.ServerReflection": {
			"ServerReflectionInfo",
		},
		"grpc.reflection.v1.ServerReflection": {
			"ServerReflectionInfo",
		},
	}
}

type ScanResult struct {
	Target            string              `json:"target"`
	AvailableServices []string            `json:"available_services"`
	HealthStatus      map[string]string   `json:"health_status,omitempty"`
	ReflectionEnabled bool                `json:"reflection_enabled"`
	MethodsFound      map[string][]string `json:"methods_found,omitempty"`
	Errors            map[string]string   `json:"errors,omitempty"`
	Vulnerabilities   []string            `json:"vulnerabilities,omitempty"`
	mutex             sync.Mutex          // Protects concurrent access to maps
}

// generateVersionedServicePaths takes a base service path and generates
// variations with different versions (v1, v2, v3, etc.)
func generateVersionedServicePaths(servicePath string, maxVersion int) []string {
	// Skip if the service path doesn't contain a version pattern
	if !strings.Contains(servicePath, ".v1.") &&
		!strings.Contains(servicePath, ".v2.") &&
		!strings.Contains(servicePath, ".v3.") {
		return []string{servicePath}
	}

	var results []string

	// Extract service name components
	for v := 1; v <= maxVersion; v++ {
		// Replace version in the service path
		versionedPath := servicePath

		// Replace v1, v2, v3 with the target version
		for i := 1; i <= maxVersion; i++ {
			versionPattern := fmt.Sprintf(".v%d.", i)
			newVersion := fmt.Sprintf(".v%d.", v)
			if strings.Contains(versionedPath, versionPattern) {
				versionedPath = strings.Replace(versionedPath, versionPattern, newVersion, 1)
				break
			}
		}

		// Only add to results if it's different from the original
		if versionedPath != servicePath {
			results = append(results, versionedPath)
		}
	}

	// Add the original service path
	results = append(results, servicePath)
	return results
}

// fuzzServiceVersions expands a list of service paths to include version variations
func fuzzServiceVersions(servicePaths []string, maxVersion int) []string {
	var expandedPaths []string
	for _, path := range servicePaths {
		variations := generateVersionedServicePaths(path, maxVersion)
		expandedPaths = append(expandedPaths, variations...)
	}
	return expandedPaths
}

// mapMethodsForVersionedService attempts to find appropriate methods for a versioned service
// by looking for the same service with a different version in the methods map
func mapMethodsForVersionedService(servicePath string, methodsMap map[string][]string) []string {
	// If the service already has methods defined, use them
	if methods, ok := methodsMap[servicePath]; ok {
		return methods
	}

	// Extract version information
	versionIndex := -1
	for i := 1; i <= 10; i++ { // Look for v1 through v10
		versionPattern := fmt.Sprintf(".v%d.", i)
		if idx := strings.Index(servicePath, versionPattern); idx >= 0 {
			versionIndex = idx
			break
		}
	}

	if versionIndex < 0 {
		// No version pattern found
		return nil
	}

	// Try to find methods for the same service with different versions
	prefix := servicePath[:versionIndex]
	suffix := servicePath[versionIndex+4:] // Skip ".vN."

	// Look for any version of this service in the methods map
	for methodServicePath, methods := range methodsMap {
		if strings.HasPrefix(methodServicePath, prefix) && strings.HasSuffix(methodServicePath, suffix) {
			// Found methods for a different version of this service
			return methods
		}
	}

	return nil
}

func main() {
	var (
		target               = flag.String("target", "localhost:50051", "gRPC server address in the format host:port")
		timeout              = flag.Int("timeout", 5, "Timeout in seconds for each request")
		concurrency          = flag.Int("concurrency", 10, "Number of concurrent requests")
		outputFile           = flag.String("output", "scan_results.json", "Output file for scan results")
		verbose              = flag.Bool("verbose", false, "Enable verbose output")
		bruteForce           = flag.Bool("brute", false, "Enable brute force mode to guess services and methods")
		wordlistFile         = flag.String("wordlist", "", "Path to wordlist file for brute forcing service/method names")
		servicesFile         = flag.String("services-file", DefaultServicesFile, "Path to file containing standard service paths")
		methodsFile          = flag.String("methods-file", DefaultMethodsFile, "Path to file containing standard methods")
		commonMethodsFile    = flag.String("common-methods-file", DefaultCommonMethodsFile, "Path to file containing common methods to try on all services")
		fuzzVersions         = flag.Bool("fuzz-versions", false, "Enable fuzzing different versions (v1, v2, v3) of services")
		fuzzMethods          = flag.Bool("fuzz-methods", false, "Enable fuzzing common methods across all services")
		methodLimit          = flag.Int("method-limit", 0, "Limit the number of methods to try per service in fuzzing mode (0 for unlimited)")
		maxVersion           = flag.Int("max-version", 3, "Maximum version number to fuzz (when using --fuzz-versions)")
		autoEnableBruteForce = flag.Bool("auto-brute", true, "Automatically enable brute force if no services are found")
	)

	flag.Parse()

	// Load standard service paths from file or use fallback
	standardServicePaths, err := loadServicePathsFromFile(*servicesFile)
	if err != nil {
		if *verbose {
			log.Printf("Warning: %v, using hardcoded defaults", err)
		}
		standardServicePaths = fallbackStandardServicePaths()
	} else if *verbose {
		log.Printf("Loaded %d service paths from %s", len(standardServicePaths), *servicesFile)
	}

	// Apply version fuzzing if enabled
	if *fuzzVersions {
		originalCount := len(standardServicePaths)
		standardServicePaths = fuzzServiceVersions(standardServicePaths, *maxVersion)
		if *verbose {
			log.Printf("Version fuzzing enabled: expanded %d services to %d variations (up to v%d)",
				originalCount, len(standardServicePaths), *maxVersion)
		}
	}

	// Load standard methods from file or use fallback
	standardMethods, err := loadMethodsFromFile(*methodsFile)
	if err != nil {
		if *verbose {
			log.Printf("Warning: %v, using hardcoded defaults", err)
		}
		standardMethods = fallbackStandardMethods()
	} else if *verbose {
		log.Printf("Loaded methods for %d services from %s", len(standardMethods), *methodsFile)
	}

	// If using custom services file, try to load custom methods
	if *servicesFile != DefaultServicesFile {
		customMethodsPath := "data/custom_methods.json"
		customMethods, err := loadMethodsFromFile(customMethodsPath)
		if err == nil {
			// Merge custom methods into standardMethods
			for service, methods := range customMethods {
				standardMethods[service] = methods
			}
			if *verbose {
				log.Printf("Loaded custom methods for %d services from %s", len(customMethods), customMethodsPath)
			}
		}
	}

	// Load common methods if method fuzzing is enabled
	var commonMethods []string
	if *fuzzMethods {
		commonMethods, err = loadCommonMethodsFromFile(*commonMethodsFile)
		if err != nil {
			if *verbose {
				log.Printf("Warning: failed to load common methods: %v, will use service-specific methods only", err)
			}
		} else if *verbose {
			log.Printf("Loaded %d common methods from %s", len(commonMethods), *commonMethodsFile)
		}
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Set up a connection to the server
	conn, err := grpc.Dial(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	if *verbose {
		log.Printf("Connected to %s", *target)
	}

	// Wait for connection to be ready
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	for {
		state := conn.GetState()
		if state == connectivity.Ready || state == connectivity.Idle {
			break
		}
		if !conn.WaitForStateChange(waitCtx, state) {
			if *verbose {
				log.Printf("Timeout waiting for connection to be ready, current state: %s", state)
			}
			break
		}
	}

	// Initialize result structure
	result := &ScanResult{
		Target:            *target,
		AvailableServices: []string{},
		HealthStatus:      make(map[string]string),
		MethodsFound:      make(map[string][]string),
		Errors:            make(map[string]string),
		Vulnerabilities:   []string{},
	}
	
	// Print scan header
	fmt.Printf("Starting gRPC scan of %s\n", *target)
	fmt.Println("=" + strings.Repeat("=", 50))

	// Check standard services (with method fuzzing if enabled)
	checkStandardServices(ctx, conn, result, standardServicePaths, standardMethods, commonMethods, *fuzzMethods, *methodLimit, *verbose)

	// Check health service specifically
	checkHealthService(ctx, conn, result, *verbose)

	// Try to use reflection to discover more services
	reflectionEnabled := false
	reflectionClient := grpc_reflection_v1alpha.NewServerReflectionClient(conn)
	reflectionStream, err := reflectionClient.ServerReflectionInfo(ctx)
	if err != nil {
		if *verbose {
			log.Printf("Reflection not available: %v", err)
		}
		result.ReflectionEnabled = false
	} else {
		// Don't assume reflection is enabled yet - need to test if it actually works
		if *verbose {
			log.Println("Server reflection stream opened, testing reflection...")
		}
		// Pass a flag to track if reflection actually worked
		actuallyWorked := discoverServicesViaReflection(ctx, reflectionStream, result, *verbose)
		result.ReflectionEnabled = actuallyWorked
		reflectionEnabled = actuallyWorked
		reflectionStream.CloseSend()
	}

	// Determine if we need to enable brute force mode automatically
	shouldBruteForce := *bruteForce
	if *autoEnableBruteForce && !reflectionEnabled && len(result.AvailableServices) == 0 {
		if *verbose {
			log.Println("Reflection not available and no services detected, automatically enabling brute force mode")
		}
		shouldBruteForce = true
	} else if *autoEnableBruteForce && len(result.AvailableServices) == 0 {
		if *verbose {
			log.Println("No services detected, automatically enabling brute force mode")
		}
		shouldBruteForce = true
	} else if *autoEnableBruteForce && !reflectionEnabled && len(result.AvailableServices) <= 1 {
		// If we only found 1 service (likely the health service) and reflection is not available
		if *verbose {
			log.Println("Reflection not available and few services detected, automatically enabling brute force mode")
		}
		shouldBruteForce = true
	}

	// Brute force services and methods if needed
	if shouldBruteForce {
		var wordlist []string
		if *wordlistFile != "" {
			data, err := os.ReadFile(*wordlistFile)
			if err != nil {
				log.Fatalf("Failed to read wordlist file: %v", err)
			}
			wordlist = strings.Split(strings.TrimSpace(string(data)), "\n")
		} else {
			// Expanded default wordlist with more common service and method components
			wordlist = []string{
				// Common service types
				"service", "api", "server", "data", "auth", "user", "admin", "config",
				"system", "metrics", "stats", "status", "debug", "internal", "public",
				"health", "ping", "echo", "test", "info", "help", "util", "utility",

				// Business domain services
				"product", "order", "payment", "cart", "basket", "item", "customer",
				"account", "profile", "notification", "message", "email", "sms",
				"file", "document", "image", "video", "media", "content",
				"search", "query", "filter", "find", "lookup", "discovery",

				// Technical services
				"cache", "database", "storage", "queue", "stream", "event", "task",
				"job", "worker", "scheduler", "cron", "time", "date", "log",
				"monitor", "trace", "metric", "analytics", "report", "dashboard",

				// Common GRPC services
				"grpc", "reflection", "health", "channelz", "instrumentation",

				// Your specific test services
				"hello", "user", "product", "ping",

				// Methods
				"create", "get", "list", "update", "delete", "watch", "stream",
				"find", "search", "query", "filter", "count", "exists", "validate",
				"verify", "check", "ping", "echo", "status", "health",
				"login", "logout", "register", "authenticate", "authorize",

				// Additional service names that might be common
				"connect", "member", "review", "cart", "basket", "event", "comment",
				"rating", "session", "subscription", "plan", "feature", "setting",
				"preference", "tag", "category", "location", "address", "payment",
				"shipping", "delivery", "invoice", "receipt", "tax", "discount",
				"promotion", "offer", "coupon", "reward", "loyalty", "point",
			}
		}

		if *verbose {
			log.Printf("Starting brute force discovery with %d wordlist entries and concurrency %d",
				len(wordlist), *concurrency)
		}

		originalServiceCount := len(result.AvailableServices)
		bruteForceServices(ctx, conn, result, wordlist, *concurrency, *verbose)

		if *verbose {
			newServices := len(result.AvailableServices) - originalServiceCount
			log.Printf("Brute force discovery found %d additional services", newServices)
		}
	}

	// Check for vulnerabilities
	checkForVulnerabilities(result)

	// Save results
	saveResults(result, *outputFile)

	// Print summary
	printSummary(result)
}

func checkStandardServices(ctx context.Context, conn *grpc.ClientConn, result *ScanResult,
	standardServicePaths []string, standardMethods map[string][]string,
	commonMethods []string, fuzzMethods bool, methodLimit int, verbose bool) {

	// Wait for connection to be ready
	state := conn.GetState()
	if state != connectivity.Ready {
		if verbose {
			log.Printf("Waiting for connection to be ready (current state: %s)", state)
		}
		// Try to establish connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn.WaitForStateChange(ctx, state)
		state = conn.GetState()
		if state != connectivity.Ready && state != connectivity.Idle {
			if verbose {
				log.Printf("Connection still not ready (state: %s), proceeding anyway", state)
			}
		}
	}

	for _, servicePath := range standardServicePaths {
		if verbose {
			log.Printf("Checking standard service: %s", servicePath)
		}

		// Get methods for this service, or find similar versioned service methods
		methods := mapMethodsForVersionedService(servicePath, standardMethods)

		if methods == nil || len(methods) == 0 {
			// If no methods found, try a basic "Get" method as fallback
			methods = []string{"Get"}

			// For versioned service, extract the service name and add standard methods
			if strings.Contains(servicePath, ".v") {
				parts := strings.Split(servicePath, ".")
				if len(parts) > 0 {
					serviceName := parts[len(parts)-1]
					// Remove "Service" suffix if present
					serviceName = strings.TrimSuffix(serviceName, "Service")
					methods = appendBasicMethodsForServiceName(serviceName, methods)
				}
			}
		}

		// If method fuzzing is enabled and we have common methods, add them
		if fuzzMethods && len(commonMethods) > 0 {
			// If method limit is set, only use that many methods
			if methodLimit > 0 && len(commonMethods) > methodLimit {
				// Add a selection of methods - prioritize basic ones that are likely to exist
				priorityMethods := getHighPriorityMethods(commonMethods, methodLimit)
				methods = append(methods, priorityMethods...)
				if verbose {
					log.Printf("Using %d common methods (limited from %d) for service: %s",
						methodLimit, len(commonMethods), servicePath)
				}
			} else {
				// Use all common methods
				methods = append(methods, commonMethods...)
				if verbose && len(commonMethods) > 0 {
					log.Printf("Using all %d common methods for service: %s",
						len(commonMethods), servicePath)
				}
			}
			// Remove duplicates from methods
			methods = uniqueStrings(methods)
		}

		// Track if service exists and its methods
		serviceFound := false
		var foundMethods []string
		serviceExplicitlyNotFound := false

		// Try each method for this service
		for _, method := range methods {
			fullMethod := fmt.Sprintf("/%s/%s", servicePath, method)

			// Attempt to invoke the method (this will usually fail, but helps detect if the service exists)
			err := conn.Invoke(ctx, fullMethod, nil, nil)
			if err != nil {
				// Parse the error status
				st, ok := status.FromError(err)

				if !ok {
					// Not a gRPC status error (likely a connection error)
					if verbose {
						log.Printf("Method not available (connection error): %s (Error: %v)", fullMethod, err)
					}
					continue
				}

				// Simplify detection logic: binary determination of service existence
				// Only consider errors that explicitly state the service doesn't exist as negative
				errMsg := strings.ToLower(st.Message())

				// Service definitely doesn't exist if the error explicitly says so
				if (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) &&
					strings.Contains(errMsg, "service") &&
					strings.Contains(errMsg, "unknown service") {
					if verbose {
						log.Printf("Service definitely doesn't exist: %s (Error: %v)", fullMethod, err)
					}
					serviceExplicitlyNotFound = true
					break
				}

				// Method doesn't exist, but service does - important to distinguish
				if st.Code() == codes.Unimplemented &&
					strings.Contains(errMsg, "method") &&
					strings.Contains(errMsg, "unknown method") {
					if verbose {
						log.Printf("Method doesn't exist but service does: %s", fullMethod)
					}
					serviceFound = true
					// Continue checking other methods
					continue
				}

				// Any other error indicates the service might exist
				// Be more conservative - require specific evidence
				if st.Code() == codes.Unavailable {
					// Skip connection issues
					if verbose {
						log.Printf("Connection issue for %s: %v", fullMethod, err)
					}
					continue
				}

				// For certain error types, we know the service exists AND method exists
				if st.Code() == codes.InvalidArgument ||
					st.Code() == codes.FailedPrecondition ||
					st.Code() == codes.OutOfRange ||
					st.Code() == codes.Unauthenticated ||
					st.Code() == codes.PermissionDenied ||
					st.Code() == codes.Internal {
					serviceFound = true
					foundMethods = append(foundMethods, method)
					if verbose {
						log.Printf("Service exists and method confirmed: %s (error: %s)", fullMethod, st.Code())
					}
				} else if verbose {
					log.Printf("Unknown error (may not indicate service exists): %s (error: %s)", fullMethod, st.Code())
				}

				// For codes.Unimplemented with method in error, method doesn't exist
				if st.Code() == codes.Unimplemented && strings.Contains(errMsg, "method") {
					if verbose {
						log.Printf("Method doesn't exist: %s", method)
					}
				} else if st.Code() == codes.Unimplemented {
					// Service exists but method doesn't
					if verbose {
						log.Printf("Service exists but method not implemented: %s", method)
					}
				}

				// Continue checking other methods
			} else {
				// Method exists and doesn't require parameters
				if verbose {
					log.Printf("Method definitely exists: %s", fullMethod)
				}

				serviceFound = true
				foundMethods = append(foundMethods, method)
				break
			}
		}

		// If service was explicitly not found, skip it
		if serviceExplicitlyNotFound {
			if verbose {
				log.Printf("Skipping service %s as it explicitly doesn't exist", servicePath)
			}
			continue
		}

		// Add to available services if found
		if serviceFound {
			if verbose {
				log.Printf("Service %s exists", servicePath)
			}

			result.mutex.Lock()
			if !contains(result.AvailableServices, servicePath) {
				result.AvailableServices = append(result.AvailableServices, servicePath)
				
				// Print immediately when we find a new service
				fmt.Printf("[+] Found service: %s\n", servicePath)
			}

			if len(foundMethods) > 0 {
				if _, ok := result.MethodsFound[servicePath]; !ok {
					result.MethodsFound[servicePath] = []string{}
				}
				oldLen := len(result.MethodsFound[servicePath])
				result.MethodsFound[servicePath] = append(result.MethodsFound[servicePath], foundMethods...)
				// Remove duplicates
				result.MethodsFound[servicePath] = uniqueStrings(result.MethodsFound[servicePath])
				
				// Print methods if we found new ones
				if len(result.MethodsFound[servicePath]) > oldLen {
					fmt.Printf("    Methods: %v\n", foundMethods)
				}
			}
			result.mutex.Unlock()
		}
	}
}

// appendBasicMethodsForServiceName generates common methods based on service name
func appendBasicMethodsForServiceName(serviceName string, methods []string) []string {
	// Add standard CRUD methods with service name
	methods = append(methods,
		fmt.Sprintf("Get%s", serviceName),
		fmt.Sprintf("List%ss", serviceName),
		fmt.Sprintf("Create%s", serviceName),
		fmt.Sprintf("Update%s", serviceName),
		fmt.Sprintf("Delete%s", serviceName))

	// For some common service types, add more specific methods
	switch strings.ToLower(serviceName) {
	case "user":
		methods = append(methods, "Login", "Logout", "Register", "VerifyEmail", "ResetPassword")
	case "auth":
		methods = append(methods, "Login", "Logout", "Refresh", "Validate", "GenerateToken")
	case "file":
		methods = append(methods, "Upload", "Download", "Delete", "List", "GetInfo")
	case "payment":
		methods = append(methods, "Process", "Refund", "GetStatus", "CalculateTotal")
	}

	return methods
}

func checkHealthService(ctx context.Context, conn *grpc.ClientConn, result *ScanResult, verbose bool) {
	healthClient := healthpb.NewHealthClient(conn)

	// Check overall health
	resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		if verbose {
			log.Printf("Health service error: %v", err)
		}
		result.mutex.Lock()
		result.Errors["health_check"] = err.Error()
		result.mutex.Unlock()
	} else {
		status := resp.GetStatus().String()

		result.mutex.Lock()
		result.HealthStatus[""] = status
		result.mutex.Unlock()

		if verbose {
			log.Printf("Overall health status: %s", status)
		}

		// Add to available services if not already present
		result.mutex.Lock()
		if !contains(result.AvailableServices, "grpc.health.v1.Health") {
			result.AvailableServices = append(result.AvailableServices, "grpc.health.v1.Health")
			fmt.Printf("[+] Found service: grpc.health.v1.Health\n")
		}

		// Add methods
		if _, ok := result.MethodsFound["grpc.health.v1.Health"]; !ok {
			result.MethodsFound["grpc.health.v1.Health"] = []string{}
		}

		if !contains(result.MethodsFound["grpc.health.v1.Health"], "Check") {
			result.MethodsFound["grpc.health.v1.Health"] = append(result.MethodsFound["grpc.health.v1.Health"], "Check")
			fmt.Printf("    Methods: [Check]\n")
		}
		result.mutex.Unlock()
	}

	// Check health for each discovered service
	result.mutex.Lock()
	servicesToCheck := make([]string, len(result.AvailableServices))
	copy(servicesToCheck, result.AvailableServices)
	result.mutex.Unlock()

	for _, serviceName := range servicesToCheck {
		resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{Service: serviceName})
		if err != nil {
			if verbose {
				log.Printf("Health check for %s error: %v", serviceName, err)
			}
		} else {
			status := resp.GetStatus().String()

			result.mutex.Lock()
			result.HealthStatus[serviceName] = status
			result.mutex.Unlock()

			if verbose {
				log.Printf("Health status for %s: %s", serviceName, status)
			}
		}
	}
}

func discoverServicesViaReflection(ctx context.Context, stream grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoClient, result *ScanResult, verbose bool) bool {
	if verbose {
		log.Println("Discovering services via reflection...")
	}

	// Safety check for nil stream
	if stream == nil {
		if verbose {
			log.Println("Reflection stream is nil, skipping reflection discovery")
		}
		return false
	}

	// List services request
	listReq := &grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
			ListServices: "",
		},
	}

	if err := stream.Send(listReq); err != nil {
		log.Printf("Failed to send reflection request: %v", err)
		return false
	}

	resp, err := stream.Recv()
	if err != nil {
		if verbose {
			log.Printf("Failed to receive reflection response: %v", err)
		}
		// Check if it's an "unknown service" error indicating reflection is not implemented
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unimplemented && strings.Contains(strings.ToLower(st.Message()), "unknown service") {
			if verbose {
				log.Printf("Server reflection is not implemented")
			}
			return false
		}
		return false
	}

	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		log.Println("Received unexpected reflection response type")
		return false
	}

	// Collect services first to avoid modifying the map during iteration
	discoveredServices := []string{}

	for _, service := range listResp.GetService() {
		serviceName := service.GetName()
		if verbose {
			log.Printf("Discovered service via reflection: %s", serviceName)
		}

		discoveredServices = append(discoveredServices, serviceName)
	}

	// Now update the result with the mutex
	if len(discoveredServices) > 0 {
		result.mutex.Lock()
		for _, serviceName := range discoveredServices {
			if !contains(result.AvailableServices, serviceName) {
				result.AvailableServices = append(result.AvailableServices, serviceName)
				fmt.Printf("[+] Found service (via reflection): %s\n", serviceName)
			}
		}
		result.mutex.Unlock()
	}

	// Try to get method information for each service
	for _, serviceName := range discoveredServices {
		// Now try to get method information for this service
		fileReq := &grpc_reflection_v1alpha.ServerReflectionRequest{
			MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: serviceName,
			},
		}

		if err := stream.Send(fileReq); err != nil {
			if verbose {
				log.Printf("Failed to send file descriptor request for %s: %v", serviceName, err)
			}
			continue
		}

		_, err := stream.Recv()
		if err != nil {
			if verbose {
				log.Printf("Failed to receive file descriptor for %s: %v", serviceName, err)
			}
			continue
		}

		// Parsing the file descriptor is complex and requires protobuf-specific code
		// For a real tool, you'd use protoreflect package to parse these
		// For now, we just note that we found the service
		if verbose {
			log.Printf("Received file descriptor for %s", serviceName)
		}
	}

	if verbose {
		log.Printf("Reflection discovery found %d services", len(discoveredServices))
	}
	
	// If we made it this far and found services, reflection is working
	return len(discoveredServices) > 0
}

func bruteForceServices(ctx context.Context, conn *grpc.ClientConn, result *ScanResult, wordlist []string, concurrency int, verbose bool) {
	if verbose {
		log.Println("Starting brute force discovery of services and methods...")
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)

	// Generate potential service names
	potentialServices := generatePotentialServiceNames(wordlist)

	if verbose {
		log.Printf("Generated %d potential service names to check", len(potentialServices))
	}

	// Use a more focused set of methods for efficiency
	focusedMethods := []string{
		"Get", "List", "Create", "Update", "Delete",
		"Check", "Ping", "Status", "Health", "Echo",
		"Login", "Verify", "Find", "Search", "Count",
	}

	for _, servicePath := range potentialServices {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(servicePath string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Local storage for methods found for this service
			var methodsFound []string
			serviceFound := false

			// First try with a minimal set of common methods for efficiency
			for _, method := range focusedMethods {
				fullMethod := fmt.Sprintf("/%s/%s", servicePath, method)

				if verbose {
					log.Printf("Trying method: %s", fullMethod)
				}

				// Try to invoke the method
				err := conn.Invoke(ctx, fullMethod, nil, nil)
				if err != nil {
					// Parse the error status
					st, ok := status.FromError(err)

					if !ok {
						// Not a gRPC status error (likely a connection error)
						if verbose {
							log.Printf("Method not available (connection error): %s (Error: %v)", fullMethod, err)
						}
						continue
					}

					// Simplify detection logic: binary determination of service existence
					// Only consider errors that explicitly state the service doesn't exist as negative
					errMsg := strings.ToLower(st.Message())

					// Service definitely doesn't exist if the error explicitly says so
					if (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) &&
						strings.Contains(errMsg, "service") &&
						(strings.Contains(errMsg, "not found") ||
							strings.Contains(errMsg, "does not exist") ||
							strings.Contains(errMsg, "doesn't exist") ||
							strings.Contains(errMsg, "unknown")) {
						if verbose {
							log.Printf("Service definitely doesn't exist: %s (Error: %v)", fullMethod, err)
						}
						// Continue to try other methods before giving up
						continue
					}

					// Any other error indicates the service likely exists but with issues
					// like wrong method name, auth issues, parameter problems, etc.
					if verbose {
						log.Printf("Service likely exists (error: %s): %s (Error: %v)",
							st.Code(), fullMethod, err)
					}

					serviceFound = true

					// For certain error types, we know the method exists
					if st.Code() == codes.InvalidArgument ||
						st.Code() == codes.FailedPrecondition ||
						st.Code() == codes.OutOfRange ||
						st.Code() == codes.Unauthenticated ||
						st.Code() == codes.PermissionDenied {
						methodsFound = append(methodsFound, method)
					}

					// Once we find evidence of a service, we can stop checking methods
					break
				} else {
					// Method exists and doesn't require parameters
					if verbose {
						log.Printf("Method definitely exists: %s", fullMethod)
					}

					serviceFound = true
					methodsFound = append(methodsFound, method)
					break
				}
			}

			// If we found the service, try more specific methods
			if serviceFound && len(methodsFound) > 0 {
				// Generate more methods that might be specific to this service type
				serviceLower := strings.ToLower(servicePath)
				var specificMethods []string

				// Extract service type from path
				serviceParts := strings.Split(servicePath, ".")
				serviceType := ""
				if len(serviceParts) > 0 {
					serviceType = strings.ToLower(serviceParts[len(serviceParts)-1])
					// Remove common suffixes
					serviceType = strings.TrimSuffix(serviceType, "service")
					serviceType = strings.TrimSuffix(serviceType, "server")
					serviceType = strings.TrimSuffix(serviceType, "api")
					serviceType = strings.TrimSpace(serviceType)
				}

				// Add methods based on service type
				if strings.Contains(serviceLower, "user") || serviceType == "user" {
					specificMethods = append(specificMethods,
						"GetUser", "CreateUser", "UpdateUser", "DeleteUser",
						"Login", "Logout", "Register", "VerifyEmail", "ResetPassword")
				} else if strings.Contains(serviceLower, "product") || serviceType == "product" {
					specificMethods = append(specificMethods,
						"GetProduct", "ListProducts", "CreateProduct", "UpdateProduct", "DeleteProduct",
						"SearchProducts", "GetProductDetails", "GetProductByID")
				} else if strings.Contains(serviceLower, "auth") || serviceType == "auth" {
					specificMethods = append(specificMethods,
						"Login", "Logout", "Authenticate", "Authorize", "ValidateToken",
						"RefreshToken", "GenerateToken", "VerifyCredentials")
				} else if strings.Contains(serviceLower, "ping") || serviceType == "ping" {
					specificMethods = append(specificMethods,
						"Ping", "Echo", "Check", "Status", "Health", "IsAlive")
				} else if strings.Contains(serviceLower, "hello") || serviceType == "hello" {
					specificMethods = append(specificMethods,
						"SayHello", "Hello", "Greet", "StreamHello")
				}

				if verbose {
					log.Printf("Service %s exists with %d confirmed methods, trying %d specific methods", 
						servicePath, len(methodsFound), len(specificMethods))
				}

				// Try the specific methods
				for _, method := range specificMethods {
					fullMethod := fmt.Sprintf("/%s/%s", servicePath, method)

					if verbose {
						log.Printf("Trying specific method: %s", fullMethod)
					}

					err := conn.Invoke(ctx, fullMethod, nil, nil)
					if err != nil {
						st, ok := status.FromError(err)
						if !ok || st.Code() == codes.Unavailable {
							continue
						}

						// Check for errors indicating method exists but with issues
						if st.Code() == codes.InvalidArgument ||
							st.Code() == codes.FailedPrecondition ||
							st.Code() == codes.Unauthenticated ||
							st.Code() == codes.PermissionDenied ||
							st.Code() == codes.Internal {
							if verbose {
								log.Printf("Method exists: %s (Code: %s)", fullMethod, st.Code())
							}
							methodsFound = append(methodsFound, method)
						}
					} else {
						if verbose {
							log.Printf("Method exists: %s", fullMethod)
						}
						methodsFound = append(methodsFound, method)
					}
				}
			}

			// Update results only if we found the service with confirmed methods
			if serviceFound && len(methodsFound) > 0 {
				// Acquire mutex before updating shared data
				result.mutex.Lock()
				defer result.mutex.Unlock()

				// Update available services
				if !contains(result.AvailableServices, servicePath) {
					result.AvailableServices = append(result.AvailableServices, servicePath)
					
					// Print immediately when we find a new service from brute force
					fmt.Printf("[+] Found service: %s\n", servicePath)
				}

				// Update methods found
				if _, ok := result.MethodsFound[servicePath]; !ok {
					result.MethodsFound[servicePath] = []string{}
				}
				oldLen := len(result.MethodsFound[servicePath])
				result.MethodsFound[servicePath] = append(result.MethodsFound[servicePath], methodsFound...)
				result.MethodsFound[servicePath] = uniqueStrings(result.MethodsFound[servicePath])

				// Print methods if we found new ones
				if len(result.MethodsFound[servicePath]) > oldLen {
					fmt.Printf("    Methods: %v\n", methodsFound)
				}

				if verbose {
					log.Printf("Added service %s with %d methods", servicePath, len(methodsFound))
				}
			}
		}(servicePath)
	}

	wg.Wait()
}

func generatePotentialServiceNames(wordlist []string) []string {
	result := make([]string, 0)

	// Common package prefixes
	prefixes := []string{
		"", // No prefix
		"grpc.",
		"api.",
		"service.",
		"rpc.",
		"pb.",
		"proto.",
		"com.",
		"io.",
		"org.",
		"net.",
		"app.",
	}

	// Common service suffixes
	suffixes := []string{
		"", // No suffix
		"Service",
		"API",
		"Server",
		"Handler",
		"Provider",
		"Manager",
		"Controller",
		"Client",
	}

	// Start with known service patterns that we want to test
	knownServices := []string{
		"grpc.reflection.v1alpha.ServerReflection",
		"grpc.reflection.v1.ServerReflection",
		"grpc.health.v1.Health",
		"grpc.gateway.example.HelloService",
		"helloworld.Greeter",
		"routeguide.RouteGuide",
		"example.HelloService",
		"example.EchoService",
		"example.UserService",
		"auth.AuthService",
		"user.UserService",
		"product.ProductService",
		"order.OrderService",
		"payment.PaymentService",
		"ping.PingService",
		"test.TestService",
	}

	result = append(result, knownServices...)

	// Generate service paths from wordlist with various patterns
	for _, word := range wordlist {
		// Skip empty words
		if word == "" {
			continue
		}

		// Capitalize first letter for service names
		wordCapitalized := strings.ToUpper(word[:1]) + word[1:]

		// 1. Simple pattern: word + suffix
		for _, suffix := range suffixes {
			if suffix == "" {
				result = append(result, word)
			} else {
				result = append(result, word+suffix)
			}
		}

		// 2. Packages: prefix + word + suffix
		for _, prefix := range prefixes {
			for _, suffix := range suffixes {
				if prefix == "" && suffix == "" {
					continue // Already added above
				}

				fullName := prefix + word
				if suffix != "" {
					fullName += suffix
				}

				result = append(result, fullName)

				// Also try with capitalized word for more formal service names
				if word != wordCapitalized {
					fullNameCapitalized := prefix + wordCapitalized
					if suffix != "" {
						fullNameCapitalized += suffix
					}
					result = append(result, fullNameCapitalized)
				}
			}
		}

		// 3. Versioned patterns
		versionedPatterns := []string{
			word + ".v1." + wordCapitalized + "Service",
			word + ".v2." + wordCapitalized + "Service",
			"v1." + word + "." + wordCapitalized + "Service",
			"v1." + wordCapitalized + "Service",
			"api." + word + ".v1." + wordCapitalized + "Service",
		}
		result = append(result, versionedPatterns...)

		// 4. Common two-level package structures
		twoLevelPrefixes := []string{
			"api.internal.",
			"app.services.",
			"internal.service.",
			"service.api.",
			"grpc.service.",
			"api.service.",
			"rpc.handlers.",
		}

		for _, prefix := range twoLevelPrefixes {
			for _, suffix := range suffixes {
				if suffix == "" {
					result = append(result, prefix+word)
					result = append(result, prefix+wordCapitalized)
				} else {
					result = append(result, prefix+word+suffix)
					result = append(result, prefix+wordCapitalized+suffix)
				}
			}
		}
	}

	// Generate domain-specific patterns
	domainPatterns := generateDomainSpecificServices()
	result = append(result, domainPatterns...)

	// Deduplicate
	return uniqueStrings(result)
}

// generateDomainSpecificServices creates service names for common application domains
func generateDomainSpecificServices() []string {
	services := []string{}

	// Authentication and Identity
	authServices := []string{
		"auth.AuthService", "auth.Authentication", "auth.Authorization",
		"identity.IdentityService", "identity.UserIdentity",
		"account.AccountService", "account.UserAccount",
		"iam.IAMService", "iam.IdentityService",
		"sso.SSOService", "oauth.OAuthService",
	}
	services = append(services, authServices...)

	// User Management
	userServices := []string{
		"user.UserService", "user.UserManagement", "user.UserProfile",
		"profile.ProfileService", "profile.UserProfile",
		"member.MemberService", "member.MemberManagement",
		"customer.CustomerService", "customer.CustomerProfile",
	}
	services = append(services, userServices...)

	// E-commerce
	ecommerceServices := []string{
		"product.ProductService", "product.ProductCatalog", "product.ProductInventory",
		"catalog.CatalogService", "catalog.ProductCatalog",
		"inventory.InventoryService", "inventory.StockManagement",
		"order.OrderService", "order.OrderManagement", "order.OrderProcessing",
		"cart.CartService", "cart.ShoppingCart", "cart.CartManagement",
		"basket.BasketService", "basket.ShoppingBasket",
		"checkout.CheckoutService", "checkout.PaymentProcessing",
		"payment.PaymentService", "payment.TransactionService",
		"shipping.ShippingService", "shipping.DeliveryService",
		"pricing.PricingService", "pricing.PriceCalculation",
	}
	services = append(services, ecommerceServices...)

	// Content & Media
	contentServices := []string{
		"content.ContentService", "content.ContentManagement",
		"media.MediaService", "media.MediaManagement",
		"file.FileService", "file.FileStorage", "file.FileManagement",
		"storage.StorageService", "storage.BlobStorage",
		"image.ImageService", "image.ImageProcessing",
		"video.VideoService", "video.VideoStreaming",
		"document.DocumentService", "document.DocumentManagement",
	}
	services = append(services, contentServices...)

	// Communication
	communicationServices := []string{
		"notification.NotificationService", "notification.NotificationDelivery",
		"email.EmailService", "email.EmailDelivery",
		"sms.SMSService", "sms.TextMessageService",
		"message.MessageService", "message.MessageDelivery",
		"chat.ChatService", "chat.ChatRoom", "chat.LiveChat",
		"push.PushNotificationService", "push.PushDelivery",
	}
	services = append(services, communicationServices...)

	// Monitoring & Operations
	monitoringServices := []string{
		"health.HealthService", "health.HealthCheck",
		"metrics.MetricsService", "metrics.MetricsCollection",
		"logging.LoggingService", "logging.LogManagement",
		"monitoring.MonitoringService", "monitoring.SystemMonitor",
		"diagnostics.DiagnosticsService", "diagnostics.SystemDiagnostics",
		"status.StatusService", "status.ServiceStatus",
		"ping.PingService", "ping.ConnectivityCheck",
	}
	services = append(services, monitoringServices...)

	// Search & Discovery
	searchServices := []string{
		"search.SearchService", "search.SearchEngine",
		"query.QueryService", "query.QueryProcessor",
		"discovery.DiscoveryService", "discovery.ServiceDiscovery",
		"lookup.LookupService", "lookup.DataLookup",
		"recommendation.RecommendationService", "recommendation.Recommender",
		"suggestion.SuggestionService", "suggestion.AutoSuggest",
	}
	services = append(services, searchServices...)

	// System & Configuration
	systemServices := []string{
		"config.ConfigService", "config.ConfigurationManagement",
		"settings.SettingsService", "settings.UserSettings",
		"system.SystemService", "system.SystemManagement",
		"platform.PlatformService", "platform.ServicePlatform",
		"registry.RegistryService", "registry.ServiceRegistry",
		"proxy.ProxyService", "proxy.GrpcProxy",
		"gateway.GatewayService", "gateway.ApiGateway",
	}
	services = append(services, systemServices...)

	return services
}

func checkForVulnerabilities(result *ScanResult) {
	// Check for potentially insecure configurations

	// 1. Check if debug/admin services are exposed
	for _, service := range result.AvailableServices {
		if strings.Contains(strings.ToLower(service), "debug") ||
			strings.Contains(strings.ToLower(service), "admin") ||
			strings.Contains(strings.ToLower(service), "internal") {
			result.Vulnerabilities = append(
				result.Vulnerabilities,
				fmt.Sprintf("Potentially sensitive service exposed: %s", service),
			)
		}
	}

	// 2. Check if reflection is enabled in production
	// Note: This is not always a vulnerability, but could be in some contexts
	if result.ReflectionEnabled {
		result.Vulnerabilities = append(
			result.Vulnerabilities,
			"Server reflection is enabled, which may expose service details",
		)
	}

	// 3. Check for non-serving health status
	for service, status := range result.HealthStatus {
		if status != "SERVING" {
			serviceName := service
			if serviceName == "" {
				serviceName = "Overall server"
			}
			result.Vulnerabilities = append(
				result.Vulnerabilities,
				fmt.Sprintf("%s reported non-serving health status: %s", serviceName, status),
			)
		}
	}
}

func saveResults(result *ScanResult, outputPath string) {
	// Create output directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal results to JSON: %v", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		log.Fatalf("Failed to write results to file: %v", err)
	}

	log.Printf("Results saved to %s", outputPath)
}

func printSummary(result *ScanResult) {
	fmt.Println("\n======== gRPC Endpoint Scanner Summary ========")
	fmt.Printf("Target: %s\n", result.Target)
	fmt.Printf("Services found: %d\n", len(result.AvailableServices))

	if len(result.AvailableServices) > 0 {
		fmt.Println("\nAvailable Services:")
		for _, service := range result.AvailableServices {
			fmt.Printf("  - %s\n", service)

			if methods, ok := result.MethodsFound[service]; ok && len(methods) > 0 {
				fmt.Println("    Methods:")
				for _, method := range methods {
					fmt.Printf("      - %s\n", method)
				}
			} else {
				fmt.Println("    No methods confirmed (service exists but no methods were validated)")
			}
		}
	}

	if len(result.HealthStatus) > 0 {
		fmt.Println("\nHealth Status:")
		for service, status := range result.HealthStatus {
			if service == "" {
				fmt.Printf("  Overall: %s\n", status)
			} else {
				fmt.Printf("  %s: %s\n", service, status)
			}
		}
	}

	if result.ReflectionEnabled {
		fmt.Println("\nServer reflection is enabled.")
	} else {
		fmt.Println("\nServer reflection is not enabled or not available.")
	}

	if len(result.Vulnerabilities) > 0 {
		fmt.Println("\nPotential Issues:")
		for _, vuln := range result.Vulnerabilities {
			fmt.Printf("  - %s\n", vuln)
		}
	}

	fmt.Println("\nDetailed results saved to JSON file.")
	fmt.Println("==============================================")
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getHighPriorityMethods returns a subset of the most important methods to try
func getHighPriorityMethods(methods []string, limit int) []string {
	// Define high priority methods that are most likely to exist
	highPriority := map[string]bool{
		"Get": true, "List": true, "Create": true, "Update": true, "Delete": true,
		"GetById": true, "GetByName": true, "Search": true, "Count": true,
		"Ping": true, "Check": true, "Status": true, "Health": true, "Version": true,
		"Login": true, "Logout": true, "Authorize": true, "Validate": true,
		"GetConfig": true, "SetConfig": true, "GetSettings": true,
	}

	// First, add high priority methods
	var result []string
	for _, method := range methods {
		if highPriority[method] && len(result) < limit {
			result = append(result, method)
		}
	}

	// If we still have room, add other methods until we reach the limit
	if len(result) < limit {
		for _, method := range methods {
			if !highPriority[method] && len(result) < limit {
				result = append(result, method)
			}
		}
	}

	return result
}

// uniqueStrings removes duplicate strings from a slice
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range input {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}
