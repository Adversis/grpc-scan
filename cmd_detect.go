package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// DetectResult represents the result of checking a single target
type DetectResult struct {
	Target    string
	IsGRPC    bool
	Error     string
	Latency   time.Duration
	Timestamp time.Time
}

// runDetectCommand handles the detect subcommand for bulk gRPC detection
func runDetectCommand(args []string) {
	if len(args) < 1 || strings.HasPrefix(args[0], "-h") || strings.HasPrefix(args[0], "--help") {
		fmt.Println("Usage: grpc-scanner detect [options]")
		fmt.Println("\nQuickly detect gRPC services on multiple targets")
		fmt.Println("\nOptions:")
		fmt.Println("  -targets string   File containing list of targets (one per line)")
		fmt.Println("  -target string    Single target to check")
		fmt.Println("  -threads int      Number of concurrent threads (default: 50)")
		fmt.Println("  -timeout int      Timeout per target in seconds (default: 3)")
		fmt.Println("  -output string    Output file for results (default: stdout)")
		fmt.Println("  -json             Output results in JSON format")
		fmt.Println("  -v                Verbose output")
		fmt.Println("\nExamples:")
		fmt.Println("  grpc-scanner detect -targets=domains.txt -threads=100")
		fmt.Println("  grpc-scanner detect -target=api.example.com:443")
		fmt.Println("  cat targets.txt | grpc-scanner detect -threads=200 -output=grpc_services.txt")
		return
	}

	var (
		targetsFile = ""
		singleTarget = ""
		threads     = 50
		timeout     = 3
		outputFile  = ""
		jsonOutput  = false
		verbose     = false
	)

	// Parse detect-specific flags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-targets=") {
			targetsFile = strings.TrimPrefix(arg, "-targets=")
		} else if strings.HasPrefix(arg, "-target=") {
			singleTarget = strings.TrimPrefix(arg, "-target=")
		} else if strings.HasPrefix(arg, "-threads=") {
			fmt.Sscanf(strings.TrimPrefix(arg, "-threads="), "%d", &threads)
		} else if strings.HasPrefix(arg, "-timeout=") {
			fmt.Sscanf(strings.TrimPrefix(arg, "-timeout="), "%d", &timeout)
		} else if strings.HasPrefix(arg, "-output=") {
			outputFile = strings.TrimPrefix(arg, "-output=")
		} else if arg == "-json" {
			jsonOutput = true
		} else if arg == "-v" {
			verbose = true
		}
	}

	// Collect targets
	var targets []string
	
	// Single target
	if singleTarget != "" {
		targets = append(targets, singleTarget)
	}
	
	// File targets
	if targetsFile != "" {
		file, err := os.Open(targetsFile)
		if err != nil {
			log.Fatalf("Failed to open targets file: %v", err)
		}
		defer file.Close()
		
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				targets = append(targets, line)
			}
		}
		
		if err := scanner.Err(); err != nil {
			log.Fatalf("Error reading targets file: %v", err)
		}
	}
	
	// Read from stdin if no targets specified
	if len(targets) == 0 && targetsFile == "" && singleTarget == "" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				targets = append(targets, line)
			}
		}
		
		if err := scanner.Err(); err != nil {
			log.Fatalf("Error reading from stdin: %v", err)
		}
	}
	
	if len(targets) == 0 {
		log.Fatal("No targets provided. Use -target, -targets, or provide input via stdin")
	}

	// Setup output
	var output *os.File
	if outputFile != "" {
		var err error
		output, err = os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to create output file: %v", err)
		}
		defer output.Close()
	} else {
		output = os.Stdout
	}

	// Start detection
	fmt.Fprintf(os.Stderr, "[*] Starting gRPC detection on %d targets with %d threads\n", len(targets), threads)
	
	results := detectGRPCServices(targets, threads, time.Duration(timeout)*time.Second, verbose)
	
	// Output results
	grpcCount := 0
	if jsonOutput {
		// JSON output format
		fmt.Fprintln(output, "[")
		for i, result := range results {
			if result.IsGRPC {
				grpcCount++
			}
			
			fmt.Fprintf(output, "  {")
			fmt.Fprintf(output, `"target":"%s",`, result.Target)
			fmt.Fprintf(output, `"is_grpc":%v,`, result.IsGRPC)
			fmt.Fprintf(output, `"latency_ms":%d,`, result.Latency.Milliseconds())
			if result.Error != "" {
				fmt.Fprintf(output, `"error":"%s",`, strings.ReplaceAll(result.Error, `"`, `\"`))
			}
			fmt.Fprintf(output, `"timestamp":"%s"`, result.Timestamp.Format(time.RFC3339))
			fmt.Fprintf(output, "}")
			if i < len(results)-1 {
				fmt.Fprintln(output, ",")
			} else {
				fmt.Fprintln(output)
			}
		}
		fmt.Fprintln(output, "]")
	} else {
		// Text output format
		for _, result := range results {
			if result.IsGRPC {
				grpcCount++
				fmt.Fprintf(output, "[+] %s - gRPC service detected (%dms)\n", 
					result.Target, result.Latency.Milliseconds())
			} else if verbose {
				if result.Error != "" {
					fmt.Fprintf(output, "[-] %s - Not gRPC: %s\n", result.Target, result.Error)
				} else {
					fmt.Fprintf(output, "[-] %s - Not gRPC\n", result.Target)
				}
			}
		}
	}
	
	// Summary
	fmt.Fprintf(os.Stderr, "\n[*] Detection complete: %d/%d targets have gRPC services\n", grpcCount, len(targets))
	if outputFile != "" {
		fmt.Fprintf(os.Stderr, "[*] Results saved to: %s\n", outputFile)
	}
}

// detectGRPCServices checks multiple targets concurrently
func detectGRPCServices(targets []string, threads int, timeout time.Duration, verbose bool) []DetectResult {
	var (
		wg          sync.WaitGroup
		resultsChan = make(chan DetectResult, len(targets))
		semaphore   = make(chan struct{}, threads)
		processed   int32
		found       int32
	)
	
	startTime := time.Now()
	total := len(targets)
	
	// Progress ticker
	if verbose {
		ticker := time.NewTicker(1 * time.Second)
		go func() {
			for range ticker.C {
				p := atomic.LoadInt32(&processed)
				f := atomic.LoadInt32(&found)
				elapsed := time.Since(startTime).Seconds()
				rate := float64(p) / elapsed
				fmt.Fprintf(os.Stderr, "\r[*] Progress: %d/%d checked (%.0f/sec) | Found: %d gRPC services", 
					p, total, rate, f)
			}
		}()
		defer ticker.Stop()
	}
	
	// Process targets concurrently
	for _, target := range targets {
		wg.Add(1)
		semaphore <- struct{}{}
		
		go func(t string) {
			defer wg.Done()
			defer func() { <-semaphore }()
			
			result := checkGRPCService(t, timeout)
			resultsChan <- result
			
			atomic.AddInt32(&processed, 1)
			if result.IsGRPC {
				atomic.AddInt32(&found, 1)
			}
		}(target)
	}
	
	// Wait for all checks to complete
	wg.Wait()
	close(resultsChan)
	
	// Collect results
	var results []DetectResult
	for result := range resultsChan {
		results = append(results, result)
	}
	
	if verbose {
		fmt.Fprintf(os.Stderr, "\r[*] Progress: %d/%d checked | Found: %d gRPC services          \n", 
			total, total, atomic.LoadInt32(&found))
	}
	
	return results
}

// checkGRPCService checks if a single target has a gRPC service
func checkGRPCService(target string, timeout time.Duration) DetectResult {
	result := DetectResult{
		Target:    target,
		Timestamp: time.Now(),
	}
	
	// Add default port if not specified
	if !strings.Contains(target, ":") {
		target = target + ":443" // Default to HTTPS port
	}
	
	startTime := time.Now()
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	// Try to connect
	conn, err := grpc.DialContext(ctx, target, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	
	if err != nil {
		result.Error = err.Error()
		result.Latency = time.Since(startTime)
		return result
	}
	defer conn.Close()
	
	// Check connection state
	state := conn.GetState()
	if state == connectivity.TransientFailure || state == connectivity.Shutdown {
		result.Error = fmt.Sprintf("Connection failed: %s", state)
		result.Latency = time.Since(startTime)
		return result
	}
	
	// Try a simple gRPC call to verify it's actually gRPC
	// Using a non-existent method to check the error response
	err = conn.Invoke(ctx, "/grpc.health.v1.Health/Check", nil, nil)
	
	if err == nil {
		// Health check succeeded - definitely gRPC
		result.IsGRPC = true
		result.Latency = time.Since(startTime)
		return result
	}
	
	// Check if it's a gRPC error
	if _, ok := status.FromError(err); ok {
		// Got a gRPC status error - this is a gRPC service
		result.IsGRPC = true
		result.Latency = time.Since(startTime)
		return result
	}
	
	// Not a gRPC error - probably not a gRPC service
	result.Error = "Non-gRPC response"
	result.Latency = time.Since(startTime)
	return result
}