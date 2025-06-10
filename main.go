package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
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

// ScanResult holds the results of a gRPC service scan
type ScanResult struct {
	Target            string              `json:"target"`
	AvailableServices []string            `json:"available_services"`
	MethodsFound      map[string][]string `json:"methods_found,omitempty"`
	ReflectionEnabled bool                `json:"reflection_enabled"`
	ScanMode          string              `json:"scan_mode"` // "reflection", "bruteforce", or "standard"
	Timestamp         string              `json:"timestamp"`
}

// Scanner encapsulates the scanning logic
type Scanner struct {
	target      string
	timeout     time.Duration
	verbose     bool
	wordlist    string
	methodsList string
	threads     int
	conn        *grpc.ClientConn
	result      *ScanResult
	resultMutex sync.Mutex
}

// Common service patterns - simplified but comprehensive
var commonPatterns = []ServicePattern{
	// Standard gRPC services
	{Service: "grpc.health.v1.Health", Methods: []string{"Check", "Watch"}},
	{Service: "grpc.reflection.v1alpha.ServerReflection", Methods: []string{"ServerReflectionInfo"}},
	{Service: "grpc.reflection.v1.ServerReflection", Methods: []string{"ServerReflectionInfo"}},

	// Common patterns - these will be expanded with smart generation
	{Service: "helloworld.Greeter", Methods: []string{"SayHello"}},
	{Service: "ping.PingService", Methods: []string{"Ping", "Check", "Health"}},
	{Service: "echo.EchoService", Methods: []string{"Echo", "Stream"}},

	// Business domain patterns
	{Service: "user", Methods: []string{"Get", "List", "Create", "Update", "Delete", "Login", "Logout"}},
	{Service: "auth", Methods: []string{"Login", "Logout", "Verify", "Refresh", "Authenticate"}},
	{Service: "product", Methods: []string{"Get", "List", "Create", "Update", "Delete", "Search"}},
	{Service: "order", Methods: []string{"Get", "List", "Create", "Update", "Delete", "Process"}},
}

// ServicePattern represents a service and its common methods
type ServicePattern struct {
	Service string
	Methods []string
}

func main() {
	// Check for subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "wordlist":
			runWordlistCommand(os.Args[2:])
			return
		case "detect":
			runDetectCommand(os.Args[2:])
			return
		}
	}

	var (
		target      = flag.String("target", "localhost:50051", "gRPC server address")
		timeout     = flag.Int("timeout", 10, "Timeout in seconds")
		output      = flag.String("output", "", "Output file for results (default: stdout)")
		verbose     = flag.Bool("v", false, "Verbose output")
		simple      = flag.Bool("simple", false, "Simple output (service names only)")
		wordlist    = flag.String("wordlist", "", "Path to wordlist file for service brute forcing")
		methodsList = flag.String("methods", "", "Path to methods wordlist (optional)")
		threads     = flag.Int("threads", 10, "Number of concurrent threads for brute forcing")
		call        = flag.String("call", "", "Call a specific method on a service (format: Service/Method or Service.Method)")
		service     = flag.String("service", "", "Test whether a specific service exists (can specify multiple with commas)")
		method      = flag.String("method", "", "Test specific methods (can specify multiple with commas)")
		help        = flag.Bool("help", false, "Show help message")
		h           = flag.Bool("h", false, "Show help message")
	)

	flag.Parse()

	// Show help if requested or no target provided
	if *help || *h || (*target == "localhost:50051" && len(os.Args) == 1) {
		fmt.Println("gRPC Scanner - A tool for discovering gRPC services and methods")
		fmt.Println("\nUsage:")
		fmt.Println("  grpc-scanner [options]                    Scan a gRPC target")
		fmt.Println("  grpc-scanner detect [options]             Detect gRPC services on multiple targets")
		fmt.Println("  grpc-scanner wordlist [options]           Generate wordlist from API docs")
		fmt.Println("\nScanning Options:")
		flag.PrintDefaults()
		fmt.Println("\nDetection Mode:")
		fmt.Println("  grpc-scanner detect -targets=domains.txt -threads=100")
		fmt.Println("  cat targets.txt | grpc-scanner detect -output=grpc_services.txt")
		fmt.Println("\nWordlist Generation:")
		fmt.Println("  grpc-scanner wordlist -url=https://api.example.com/docs -output=wordlist.txt")
		fmt.Println("  grpc-scanner wordlist -input=api_docs.html -output=wordlist.txt")
		fmt.Println("\nExamples:")
		fmt.Println("  grpc-scanner -target=api.example.com:443")
		fmt.Println("  grpc-scanner -target=api.example.com:443 -wordlist=data/grpc_wordlist.txt")
		fmt.Println("  grpc-scanner detect -targets=domains.txt -threads=200 -output=grpc_targets.txt")
		fmt.Println("\nDirect Testing (no protobuf files needed!):")
		fmt.Println("  grpc-scanner -target=api.example.com:443 -call=UserService/GetUser")
		fmt.Println("  grpc-scanner -target=api.example.com:443 -service=UserService -method=GetUser,ListUsers")
		fmt.Println("  grpc-scanner -target=api.example.com:443 -service=UserService,AuthService")
		return
	}

	// Handle direct call mode
	if *call != "" {
		handleDirectCall(*target, *call, time.Duration(*timeout)*time.Second, *verbose)
		return
	}

	// Create scanner
	scanner := &Scanner{
		target:      *target,
		timeout:     time.Duration(*timeout) * time.Second,
		verbose:     *verbose,
		wordlist:    *wordlist,
		methodsList: *methodsList,
		threads:     *threads,
		result: &ScanResult{
			Target:            *target,
			AvailableServices: []string{},
			MethodsFound:      make(map[string][]string),
			Timestamp:         time.Now().Format(time.RFC3339),
		},
	}

	// Handle direct service/method testing
	if *service != "" || *method != "" {
		scanner.handleDirectTesting(*service, *method)
		scanner.PrintResults()
		return
	}

	// Run scan
	if err := scanner.Run(); err != nil {
		log.Fatalf("Scan failed: %v", err)
	}

	// Output results
	if *output != "" {
		scanner.SaveResults(*output)
	} else if *simple {
		scanner.PrintSimple()
	} else {
		scanner.PrintResults()
	}
}

// Run executes the scan
func (s *Scanner) Run() error {
	// Connect to server
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	fmt.Printf("[+] Scanning %s...\n", s.target)

	conn, err := grpc.DialContext(ctx, s.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer conn.Close()
	s.conn = conn

	// Wait for connection
	if !s.waitForConnection(ctx) {
		fmt.Printf("[-] Failed to establish gRPC connection to %s\n", s.target)
		fmt.Println("   This may not be a gRPC service or the server is not responding")
		return fmt.Errorf("connection failed")
	}

	// Test if this is actually a gRPC service
	isGRPC, serviceType := s.detectServiceType(ctx)
	if !isGRPC {
		fmt.Printf("[!] %s does not appear to be a gRPC service\n", s.target)
		fmt.Printf("   Detected: %s\n", serviceType)
		return fmt.Errorf("not a gRPC service")
	}

	fmt.Printf("[+] Connected to gRPC service at %s\n", s.target)
	if s.verbose {
		fmt.Printf("   Connection state: %s\n", s.conn.GetState())
	}

	// Try reflection first
	if s.tryReflection(ctx) {
		s.result.ScanMode = "reflection"
		if s.verbose {
			log.Println("Using reflection for service discovery")
		}
	}

	// Always check standard services
	fmt.Println("\n[+] Checking standard gRPC services...")
	s.checkStandardServices(ctx)

	// If no services found or reflection not available, use brute force
	if !s.result.ReflectionEnabled || len(s.result.AvailableServices) <= 1 {
		if s.wordlist != "" {
			fmt.Printf("\n[+] Loading wordlist from: %s\n", s.wordlist)
			s.result.ScanMode = "wordlist"
			if err := s.wordlistBruteForce(ctx); err != nil {
				return fmt.Errorf("wordlist brute force failed: %v", err)
			}
		} else {
			fmt.Println("\n[+] Using smart pattern matching...")
			s.result.ScanMode = "bruteforce"
			s.smartBruteForce(ctx)
		}
	} else if s.result.ScanMode == "" {
		s.result.ScanMode = "standard"
	}

	return nil
}

// waitForConnection waits for the gRPC connection to be ready
func (s *Scanner) waitForConnection(ctx context.Context) bool {
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for {
		state := s.conn.GetState()
		if state == connectivity.Ready || state == connectivity.Idle {
			return true
		}
		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			return false
		}
		if !s.conn.WaitForStateChange(waitCtx, state) {
			if s.verbose {
				log.Printf("Connection state: %s", state)
			}
			return state == connectivity.Ready || state == connectivity.Idle
		}
	}
}

// detectServiceType attempts to determine if the endpoint is a gRPC service
func (s *Scanner) detectServiceType(ctx context.Context) (bool, string) {
	// Try a simple gRPC call to test if it's a gRPC service
	err := s.conn.Invoke(ctx, "/grpc.health.v1.Health/Check", nil, nil)
	if err == nil {
		return true, "gRPC service with health check"
	}

	// Check the error to determine service type
	st, ok := status.FromError(err)
	if !ok {
		// Not a gRPC error - might be HTTP or other protocol
		errStr := err.Error()
		if strings.Contains(errStr, "HTTP") {
			return false, "HTTP/REST service"
		}
		if strings.Contains(errStr, "connection refused") {
			return false, "No service listening on this port"
		}
		return false, "Unknown service type"
	}

	// It's a gRPC error - this confirms it's a gRPC service
	switch st.Code() {
	case codes.Unimplemented:
		if strings.Contains(st.Message(), "unknown service") {
			return true, "gRPC service (health check not implemented)"
		}
		return true, "gRPC service"
	case codes.Unavailable:
		return false, "Service unavailable"
	case codes.Internal:
		// Some gRPC services return Internal for unimplemented services
		return true, "gRPC service"
	default:
		// Any other gRPC error code means it's a gRPC service
		return true, "gRPC service"
	}
}

// tryReflection attempts to use server reflection for service discovery
func (s *Scanner) tryReflection(ctx context.Context) bool {
	client := grpc_reflection_v1alpha.NewServerReflectionClient(s.conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return false
	}
	defer stream.CloseSend()

	// Request service list
	req := &grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{
			ListServices: "",
		},
	}

	if err := stream.Send(req); err != nil {
		return false
	}

	resp, err := stream.Recv()
	if err != nil {
		return false
	}

	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		return false
	}

	// Process discovered services
	s.result.ReflectionEnabled = true
	for _, service := range listResp.GetService() {
		s.addService(service.GetName(), "reflection")
	}

	return true
}

// checkStandardServices checks for common gRPC services
func (s *Scanner) checkStandardServices(ctx context.Context) {
	// Check health service
	healthClient := healthpb.NewHealthClient(s.conn)
	if _, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{}); err == nil {
		s.addService("grpc.health.v1.Health", "standard")
		s.addMethod("grpc.health.v1.Health", "Check")
	}

	// Check a few other standard patterns
	for _, pattern := range commonPatterns[:3] { // Just the standard gRPC services
		if s.checkService(ctx, pattern.Service, pattern.Methods[0]) {
			s.addService(pattern.Service, "standard")
			for _, method := range pattern.Methods {
				if s.checkMethod(ctx, pattern.Service, method) {
					s.addMethod(pattern.Service, method)
				}
			}
		}
	}
}

// WordlistEntry represents a service with optional specific methods
type WordlistEntry struct {
	Service string
	Methods []string
}

// loadEnhancedWordlist reads services and methods from an enhanced wordlist file
func (s *Scanner) loadEnhancedWordlist(path string) ([]WordlistEntry, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open wordlist: %v", err)
	}
	defer file.Close()

	var entries []WordlistEntry
	var globalMethods []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Check for method-only entries (start with *)
		if strings.HasPrefix(line, "*") {
			method := strings.TrimPrefix(line, "*")
			globalMethods = append(globalMethods, method)
			continue
		}

		// Check for service:methods format
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			service := strings.TrimSpace(parts[0])
			methodList := strings.Split(parts[1], ",")

			var methods []string
			for _, m := range methodList {
				if method := strings.TrimSpace(m); method != "" {
					methods = append(methods, method)
				}
			}

			entries = append(entries, WordlistEntry{
				Service: service,
				Methods: methods,
			})
		} else {
			// Simple service name
			entries = append(entries, WordlistEntry{
				Service: line,
				Methods: nil, // Will use default methods
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading wordlist: %v", err)
	}

	return entries, globalMethods, nil
}

// wordlistBruteForce performs service discovery using a wordlist
func (s *Scanner) wordlistBruteForce(ctx context.Context) error {
	// Load enhanced wordlist
	entries, globalMethods, err := s.loadEnhancedWordlist(s.wordlist)
	if err != nil {
		return err
	}

	// Default methods if none specified
	defaultMethods := []string{
		"Get", "List", "Create", "Update", "Delete",
		"Find", "Search", "Query", "Check", "Ping",
	}

	// Combine default methods with global methods from wordlist
	if len(globalMethods) > 0 {
		defaultMethods = append(defaultMethods, globalMethods...)
		// Remove duplicates
		methodSet := make(map[string]bool)
		for _, m := range defaultMethods {
			methodSet[m] = true
		}
		defaultMethods = nil
		for m := range methodSet {
			defaultMethods = append(defaultMethods, m)
		}
	}

	fmt.Printf("[+] Loaded %d service entries from wordlist\n", len(entries))
	if len(globalMethods) > 0 {
		fmt.Printf("[+] Loaded %d global methods\n", len(globalMethods))
	}
	fmt.Printf("[+] Using %d threads for parallel scanning\n", s.threads)

	// Progress tracking
	checked := 0
	found := 0
	startTime := time.Now()

	// Use goroutines for parallel checking
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.threads)

	for _, entry := range entries {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(e WordlistEntry) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Determine which methods to use
			methodsToTry := e.Methods
			if len(methodsToTry) == 0 {
				methodsToTry = defaultMethods
			}

			// Try the service name with various patterns
			patterns := []string{
				e.Service, // Raw name as provided
			}

			// Only generate patterns if it's not already a full service name
			if !strings.Contains(e.Service, ".") && !strings.HasSuffix(e.Service, "Service") {
				patterns = append(patterns,
					fmt.Sprintf("%sService", e.Service),                                   // ServiceName pattern
					fmt.Sprintf("%s.%sService", strings.ToLower(e.Service), e.Service),    // package.Service
					fmt.Sprintf("api.%s", e.Service),                                      // api.Service
					fmt.Sprintf("%s.v1.%sService", strings.ToLower(e.Service), e.Service), // versioned
				)
			}

			for _, pattern := range patterns {
				// Try with first method to check if service exists
				if len(methodsToTry) > 0 && s.checkService(ctx, pattern, methodsToTry[0]) {
					s.addService(pattern, "wordlist")
					found++

					// Check all specified methods for this service
					methodCount := 0
					for _, method := range methodsToTry {
						if s.checkMethod(ctx, pattern, method) {
							s.addMethod(pattern, method)
							methodCount++
						}
					}

					if s.verbose && methodCount > 0 {
						fmt.Printf("   └─ %d/%d methods confirmed\n", methodCount, len(methodsToTry))
					}
					break // Found this service, no need to try other patterns
				}
			}

			checked++
			if checked%50 == 0 || checked == len(entries) {
				elapsed := time.Since(startTime).Seconds()
				rate := float64(checked) / elapsed
				fmt.Printf("\r[+] Progress: %d/%d checked (%.0f/sec) | Found: %d services",
					checked, len(entries), rate, found)
			}
		}(entry)
	}

	wg.Wait()

	// Clear the progress line
	fmt.Printf("\r[+] Completed: %d/%d checked | Found: %d services          \n",
		len(entries), len(entries), len(s.result.AvailableServices))

	return nil
}

// smartBruteForce performs intelligent service discovery
func (s *Scanner) smartBruteForce(ctx context.Context) {
	patterns := s.generateSmartPatterns()

	// Use goroutines for parallel checking
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.threads) // Use configurable threads

	for _, pattern := range patterns {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(p ServicePattern) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Quick check with first method
			if s.checkService(ctx, p.Service, p.Methods[0]) {
				s.addService(p.Service, "bruteforce")

				// Check all methods for this service
				for _, method := range p.Methods {
					if s.checkMethod(ctx, p.Service, method) {
						s.addMethod(p.Service, method)
					}
				}
			}
		}(pattern)
	}

	wg.Wait()
}

// generateSmartPatterns creates service patterns based on common naming conventions
func (s *Scanner) generateSmartPatterns() []ServicePattern {
	var patterns []ServicePattern

	// Start with common patterns
	patterns = append(patterns, commonPatterns...)

	// Common service names
	serviceNames := []string{
		"User", "Auth", "Account", "Profile",
		"Product", "Order", "Payment", "Cart",
		"File", "Storage", "Media", "Document",
		"Notification", "Email", "Message",
		"Search", "Query", "Config", "Settings",
		"Admin", "Management", "System",
	}

	// Common method names
	commonMethods := []string{
		"Get", "List", "Create", "Update", "Delete",
		"Find", "Search", "Count", "Exists",
		"Validate", "Check", "Verify",
	}

	// Generate patterns with common structures
	for _, name := range serviceNames {
		// Simple name
		patterns = append(patterns, ServicePattern{
			Service: strings.ToLower(name),
			Methods: s.generateMethodsForService(name, commonMethods),
		})

		// Service suffix
		patterns = append(patterns, ServicePattern{
			Service: fmt.Sprintf("%sService", name),
			Methods: s.generateMethodsForService(name, commonMethods),
		})

		// Package.Service pattern
		patterns = append(patterns, ServicePattern{
			Service: fmt.Sprintf("%s.%sService", strings.ToLower(name), name),
			Methods: s.generateMethodsForService(name, commonMethods),
		})

		// API pattern
		patterns = append(patterns, ServicePattern{
			Service: fmt.Sprintf("api.%s", name),
			Methods: s.generateMethodsForService(name, commonMethods),
		})

		// Versioned pattern
		patterns = append(patterns, ServicePattern{
			Service: fmt.Sprintf("%s.v1.%sService", strings.ToLower(name), name),
			Methods: s.generateMethodsForService(name, commonMethods),
		})
	}

	return s.deduplicatePatterns(patterns)
}

// generateMethodsForService creates method names based on service type
func (s *Scanner) generateMethodsForService(serviceName string, baseMethods []string) []string {
	methods := make([]string, 0, len(baseMethods)+5)

	// Add base methods
	methods = append(methods, baseMethods...)

	// Add service-specific methods
	switch strings.ToLower(serviceName) {
	case "user", "account":
		methods = append(methods, "Login", "Logout", "Register", "Authenticate", "GetProfile")
	case "auth":
		methods = append(methods, "Login", "Logout", "Verify", "Refresh", "ValidateToken")
	case "file", "storage":
		methods = append(methods, "Upload", "Download", "Delete", "GetMetadata")
	case "payment":
		methods = append(methods, "Process", "Refund", "GetStatus", "Capture")
	}

	// Add Get<ServiceName> pattern
	methods = append(methods, "Get"+serviceName)

	return methods
}

// checkService checks if a service exists by trying a method
func (s *Scanner) checkService(ctx context.Context, service, method string) bool {
	fullMethod := fmt.Sprintf("/%s/%s", service, method)
	err := s.conn.Invoke(ctx, fullMethod, nil, nil)

	if err == nil {
		return true
	}

	// Analyze error to determine if service exists
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	// Service doesn't exist
	if st.Code() == codes.Unimplemented && strings.Contains(strings.ToLower(st.Message()), "unknown service") {
		return false
	}

	// Method doesn't exist but service does
	if st.Code() == codes.Unimplemented && strings.Contains(strings.ToLower(st.Message()), "unknown method") {
		return true
	}

	// Other errors suggest service exists
	switch st.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition,
		codes.Unauthenticated, codes.PermissionDenied,
		codes.Internal:
		return true
	}

	return false
}

// checkMethod checks if a specific method exists
func (s *Scanner) checkMethod(ctx context.Context, service, method string) bool {
	fullMethod := fmt.Sprintf("/%s/%s", service, method)
	err := s.conn.Invoke(ctx, fullMethod, nil, nil)

	if err == nil {
		return true
	}

	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	// Method exists if we get parameter/auth errors
	switch st.Code() {
	case codes.InvalidArgument, codes.FailedPrecondition,
		codes.Unauthenticated, codes.PermissionDenied,
		codes.Internal:
		return true
	}

	return false
}

// Thread-safe result updates
func (s *Scanner) addService(service, source string) {
	s.resultMutex.Lock()
	defer s.resultMutex.Unlock()

	// Check if already exists
	for _, existing := range s.result.AvailableServices {
		if existing == service {
			return
		}
	}

	s.result.AvailableServices = append(s.result.AvailableServices, service)
	if source != "reflection" && source != "standard" {
		// Don't print for reflection/standard as they're shown differently
		fmt.Printf("[+] Found: %s\n", service)
	}
}

func (s *Scanner) addMethod(service, method string) {
	s.resultMutex.Lock()
	defer s.resultMutex.Unlock()

	if s.result.MethodsFound[service] == nil {
		s.result.MethodsFound[service] = []string{}
	}

	// Check if already exists
	for _, existing := range s.result.MethodsFound[service] {
		if existing == method {
			return
		}
	}

	s.result.MethodsFound[service] = append(s.result.MethodsFound[service], method)
}

// deduplicatePatterns removes duplicate service patterns
func (s *Scanner) deduplicatePatterns(patterns []ServicePattern) []ServicePattern {
	seen := make(map[string]bool)
	result := []ServicePattern{}

	for _, p := range patterns {
		if !seen[p.Service] {
			seen[p.Service] = true
			result = append(result, p)
		}
	}

	return result
}

// Output methods
func (s *Scanner) PrintResults() {
	fmt.Printf("\n[+] Scan Results\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Target:          %s\n", s.result.Target)
	fmt.Printf("Discovery Mode:  %s\n", s.result.ScanMode)
	fmt.Printf("Services Found:  %d\n", len(s.result.AvailableServices))

	if s.result.ReflectionEnabled {
		fmt.Printf("Reflection:      Enabled\n")
	} else {
		fmt.Printf("Reflection:      Disabled\n")
	}

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	fmt.Println("Discovered Services:")
	for _, service := range s.result.AvailableServices {
		fmt.Printf("\n%s\n", service)
		if methods, ok := s.result.MethodsFound[service]; ok && len(methods) > 0 {
			fmt.Printf("   Methods (%d):\n", len(methods))
			for _, method := range methods {
				fmt.Printf("   └─ %s\n", method)
			}
		} else {
			fmt.Printf("   Methods: None confirmed\n")
		}
	}

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

func (s *Scanner) PrintSimple() {
	for _, service := range s.result.AvailableServices {
		fmt.Println(service)
	}
}

func (s *Scanner) SaveResults(filename string) {
	data, err := json.MarshalIndent(s.result, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal results: %v", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		log.Fatalf("Failed to save results: %v", err)
	}

	if s.verbose {
		log.Printf("Results saved to %s", filename)
	}
}

// handleDirectCall handles the -call flag for direct method invocation
func handleDirectCall(target, call string, timeout time.Duration, verbose bool) {
	// Parse the call format (Service/Method or Service.Method)
	var service, method string
	if strings.Contains(call, "/") {
		parts := strings.SplitN(call, "/", 2)
		service = parts[0]
		if len(parts) > 1 {
			method = parts[1]
		}
	} else if strings.Contains(call, ".") {
		// Find the last dot to separate service and method
		lastDot := strings.LastIndex(call, ".")
		service = call[:lastDot]
		method = call[lastDot+1:]
	} else {
		log.Fatalf("Invalid call format. Use Service/Method or Service.Method")
	}

	if method == "" {
		log.Fatalf("Method name is required in call format")
	}

	// Connect to the server
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	fmt.Printf("[+] Testing %s/%s on %s\n", service, method, target)

	// Try to invoke the method
	fullMethod := fmt.Sprintf("/%s/%s", service, method)
	err = conn.Invoke(ctx, fullMethod, nil, nil)

	if err == nil {
		fmt.Printf("[+] Success: Method exists (may require proper request message)\n")
		return
	}

	// Analyze the error
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unimplemented:
			if strings.Contains(strings.ToLower(st.Message()), "unknown service") {
				fmt.Printf("[-] Service '%s' not found\n", service)
			} else if strings.Contains(strings.ToLower(st.Message()), "unknown method") {
				fmt.Printf("[+] Service '%s' exists but method '%s' not found\n", service, method)
			} else {
				fmt.Printf("[-] Unimplemented: %v\n", st.Message())
			}
		case codes.InvalidArgument:
			fmt.Printf("[+] Method exists! (requires proper request message)\n")
			fmt.Printf("    Error: %v\n", st.Message())
		case codes.Unauthenticated:
			fmt.Printf("[+] Method exists but requires authentication\n")
			fmt.Printf("    Error: %v\n", st.Message())
		case codes.PermissionDenied:
			fmt.Printf("[+] Method exists but access denied\n")
			fmt.Printf("    Error: %v\n", st.Message())
		case codes.Internal:
			fmt.Printf("[?] Internal error (method may exist)\n")
			fmt.Printf("    Error: %v\n", st.Message())
		default:
			fmt.Printf("[?] Error: %v (code: %v)\n", st.Message(), st.Code())
		}
	} else {
		fmt.Printf("[-] Non-gRPC error: %v\n", err)
	}
}

// handleDirectTesting handles the -service and -method flags
func (s *Scanner) handleDirectTesting(services, methods string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	fmt.Printf("[+] Direct testing on %s...\n", s.target)

	// Connect to server
	conn, err := grpc.DialContext(ctx, s.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	s.conn = conn

	// Wait for connection
	if !s.waitForConnection(ctx) {
		log.Fatalf("Failed to establish gRPC connection")
	}

	// Parse services and methods
	serviceList := []string{}
	methodList := []string{}

	if services != "" {
		serviceList = strings.Split(services, ",")
		for i := range serviceList {
			serviceList[i] = strings.TrimSpace(serviceList[i])
		}
	}

	if methods != "" {
		methodList = strings.Split(methods, ",")
		for i := range methodList {
			methodList[i] = strings.TrimSpace(methodList[i])
		}
	}

	s.result.ScanMode = "direct"

	// If only methods specified, try common service patterns
	if len(serviceList) == 0 && len(methodList) > 0 {
		fmt.Println("[+] No services specified, will try common patterns with provided methods")
		// Generate common patterns
		for _, pattern := range commonPatterns {
			serviceList = append(serviceList, pattern.Service)
		}
	}

	// Test each service
	for _, service := range serviceList {
		if service == "" {
			continue
		}

		// If no methods specified, try common methods
		testMethods := methodList
		if len(testMethods) == 0 {
			testMethods = []string{"Get", "List", "Create", "Update", "Delete", "Check", "Ping"}
		}

		// Test the service with first method
		if s.checkService(ctx, service, testMethods[0]) {
			s.addService(service, "direct")

			// Test all methods
			for _, method := range testMethods {
				if s.checkMethod(ctx, service, method) {
					s.addMethod(service, method)
					if s.verbose {
						fmt.Printf("   [+] %s/%s exists\n", service, method)
					}
				} else if s.verbose {
					fmt.Printf("   [-] %s/%s not found\n", service, method)
				}
			}
		} else if s.verbose {
			fmt.Printf("   [-] Service '%s' not found\n", service)
		}
	}
}
