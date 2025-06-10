package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// WordlistExtractor extracts potential service names from API documentation
type WordlistExtractor struct {
	services       map[string]bool
	operations     map[string]bool
	resources      map[string]bool
	serviceMethods map[string]map[string]bool
}

func NewWordlistExtractor() *WordlistExtractor {
	return &WordlistExtractor{
		services:       make(map[string]bool),
		operations:     make(map[string]bool),
		resources:      make(map[string]bool),
		serviceMethods: make(map[string]map[string]bool),
	}
}

// Common patterns to identify API endpoints and services
var (
	// URL path patterns
	wlPathPattern = regexp.MustCompile(`/api/v?\d*/([a-zA-Z]+)/?`)
	// Method patterns like getUserInfo, createNote
	wlMethodPattern = regexp.MustCompile(`(get|create|update|delete|list|find|search)([A-Z][a-zA-Z]+)`)
	// Service class patterns
	wlServicePattern = regexp.MustCompile(`([A-Z][a-zA-Z]+)(Service|Client|API|Handler|Controller)`)
	// Resource patterns
	wlResourcePattern = regexp.MustCompile(`\b([A-Z][a-z]+(?:[A-Z][a-z]+)*)\b`)
)

func (e *WordlistExtractor) ExtractFromURL(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to parse HTML: %v", err)
	}

	e.extractFromNode(doc)
	return nil
}

func (e *WordlistExtractor) extractFromNode(n *html.Node) {
	if n.Type == html.TextNode {
		e.extractFromText(n.Data)
	}

	// Look for code blocks, API references
	if n.Type == html.ElementNode {
		if n.Data == "code" || n.Data == "pre" {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				e.extractFromCode(n.FirstChild.Data)
			}
		}
	}

	// Recursively process children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		e.extractFromNode(c)
	}
}

func (e *WordlistExtractor) extractFromText(text string) {
	// Extract from URL paths like /api/users/create
	pathMatches := regexp.MustCompile(`/api/v?\d*/([a-zA-Z]+)/([a-zA-Z]+)`).FindAllStringSubmatch(text, -1)
	for _, match := range pathMatches {
		if len(match) > 2 {
			service := strings.Title(match[1])
			method := strings.Title(match[2])
			e.services[service] = true
			e.addServiceMethod(service, method)
		}
	}

	// Extract from URL paths (single level)
	matches := wlPathPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			service := strings.Title(match[1])
			e.services[service] = true
		}
	}

	// Extract from method names like getUserInfo
	matches = wlMethodPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 2 {
			operation := strings.Title(match[1])
			resource := match[2]
			e.operations[operation] = true
			e.resources[resource] = true
			e.services[resource] = true
			
			// Map method to service
			e.addServiceMethod(resource, operation+resource)
		}
	}

	// Extract service patterns
	matches = wlServicePattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			service := match[1]
			e.services[service] = true
		}
	}
}

func (e *WordlistExtractor) extractFromCode(code string) {
	// More aggressive extraction from code blocks
	lines := strings.Split(code, "\n")
	currentClass := ""
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Look for class definitions
		if strings.Contains(line, "class") || strings.Contains(line, "interface") {
			classMatch := regexp.MustCompile(`(?:class|interface)\s+([A-Z][a-zA-Z]+)`).FindStringSubmatch(line)
			if len(classMatch) > 1 {
				currentClass = classMatch[1]
				e.services[currentClass] = true
			}
		}

		// Look for method definitions within a class
		if currentClass != "" {
			// Match function/method definitions
			methodMatch := regexp.MustCompile(`(?:function|func|def|public|private|protected)?\s*([a-zA-Z]+)\s*\(`).FindStringSubmatch(line)
			if len(methodMatch) > 1 {
				method := methodMatch[1]
				if method != "function" && method != "func" && method != "def" {
					e.addServiceMethod(currentClass, strings.Title(method))
				}
			}
		}

		// Look for API endpoints
		if strings.Contains(line, "/api/") || strings.Contains(line, "endpoint") {
			e.extractFromText(line)
		}
		
		// Reset current class on closing brace at start of line
		if strings.HasPrefix(line, "}") {
			currentClass = ""
		}
	}
}

// Helper method to add service-method mapping
func (e *WordlistExtractor) addServiceMethod(service, method string) {
	if e.serviceMethods[service] == nil {
		e.serviceMethods[service] = make(map[string]bool)
	}
	e.serviceMethods[service][method] = true
}

func (e *WordlistExtractor) ExtractFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		e.extractFromText(line)
		e.extractFromCode(line)
	}

	return scanner.Err()
}

// WriteEnhancedWordlist writes the wordlist in enhanced format with methods
func (e *WordlistExtractor) WriteEnhancedWordlist(writer *bufio.Writer, addPatterns bool) {
	fmt.Fprintf(writer, "# Enhanced wordlist generated from API documentation\n")
	fmt.Fprintf(writer, "# Format: ServiceName:method1,method2,method3\n")
	fmt.Fprintf(writer, "# Services without methods will use default scanning\n\n")
	
	// First, write services with specific methods
	servicesWithMethods := make([]string, 0)
	for service := range e.serviceMethods {
		if len(e.serviceMethods[service]) > 0 {
			servicesWithMethods = append(servicesWithMethods, service)
		}
	}
	sort.Strings(servicesWithMethods)
	
	fmt.Fprintf(writer, "# Services with extracted methods\n")
	for _, service := range servicesWithMethods {
		methods := make([]string, 0)
		for method := range e.serviceMethods[service] {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		
		// Write service with methods
		fmt.Fprintf(writer, "%s:%s\n", service, strings.Join(methods, ","))
		
		// Add pattern variations if requested
		if addPatterns && !strings.Contains(service, ".") && !strings.HasSuffix(service, "Service") {
			fmt.Fprintf(writer, "%sService:%s\n", service, strings.Join(methods, ","))
		}
	}
	
	// Write services without specific methods
	servicesWithoutMethods := make([]string, 0)
	for service := range e.services {
		if _, hasMethods := e.serviceMethods[service]; !hasMethods {
			servicesWithoutMethods = append(servicesWithoutMethods, service)
		}
	}
	
	if len(servicesWithoutMethods) > 0 {
		fmt.Fprintf(writer, "\n# Services without specific methods (will use defaults)\n")
		sort.Strings(servicesWithoutMethods)
		for _, service := range servicesWithoutMethods {
			fmt.Fprintf(writer, "%s\n", service)
			
			// Add pattern variations if requested
			if addPatterns && !strings.Contains(service, ".") && !strings.HasSuffix(service, "Service") {
				fmt.Fprintf(writer, "%sService\n", service)
			}
		}
	}
	
	// Write global methods
	globalMethods := make([]string, 0)
	for method := range e.operations {
		globalMethods = append(globalMethods, method)
	}
	
	// Add common gRPC methods
	commonMethods := []string{
		"Get", "GetById", "GetByName",
		"List", "ListAll",
		"Create", "CreateBatch",
		"Update", "UpdatePartial",
		"Delete", "DeleteBatch",
		"Search", "Query", "Find",
		"Stream", "Watch",
		"Validate", "Check",
	}
	
	for _, method := range commonMethods {
		e.operations[method] = true
	}
	
	// Get unique sorted list
	for method := range e.operations {
		if !stringInSlice(globalMethods, method) {
			globalMethods = append(globalMethods, method)
		}
	}
	sort.Strings(globalMethods)
	
	if len(globalMethods) > 0 {
		fmt.Fprintf(writer, "\n# Global methods to try on all services\n")
		for _, method := range globalMethods {
			fmt.Fprintf(writer, "*%s\n", method)
		}
	}
	
	// Summary
	fmt.Fprintf(writer, "\n# Summary:\n")
	fmt.Fprintf(writer, "# - %d services with specific methods\n", len(servicesWithMethods))
	fmt.Fprintf(writer, "# - %d services without methods\n", len(servicesWithoutMethods))
	fmt.Fprintf(writer, "# - %d global methods\n", len(globalMethods))
}

func stringInSlice(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// runWordlistCommand handles the wordlist subcommand
func runWordlistCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: grpc-scanner wordlist [options]")
		fmt.Println("\nGenerate wordlists from API documentation")
		fmt.Println("\nOptions:")
		fmt.Println("  -url string     URL of API documentation to extract from")
		fmt.Println("  -input string   Local file to extract from")
		fmt.Println("  -output string  Output wordlist file (default: api_wordlist.txt)")
		fmt.Println("  -enhanced       Generate enhanced format with methods (default: true)")
		fmt.Println("  -patterns       Add common gRPC patterns (default: true)")
		fmt.Println("  -v              Verbose output")
		fmt.Println("\nExample:")
		fmt.Println("  grpc-scanner wordlist -url=https://api.example.com/docs -output=wordlist.txt")
		return
	}

	var (
		url      string
		input    string
		output   = "api_wordlist.txt"
		enhanced = true
		patterns = true
		verbose  = false
	)

	// Parse wordlist-specific flags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-url=") {
			url = strings.TrimPrefix(arg, "-url=")
		} else if strings.HasPrefix(arg, "-input=") {
			input = strings.TrimPrefix(arg, "-input=")
		} else if strings.HasPrefix(arg, "-output=") {
			output = strings.TrimPrefix(arg, "-output=")
		} else if arg == "-enhanced=false" {
			enhanced = false
		} else if arg == "-patterns=false" {
			patterns = false
		} else if arg == "-v" {
			verbose = true
		}
	}

	if url == "" && input == "" {
		log.Fatal("Please provide either -url or -input")
	}

	extractor := NewWordlistExtractor()

	// Extract from URL or file
	if url != "" {
		if verbose {
			log.Printf("Extracting from URL: %s", url)
		}
		if err := extractor.ExtractFromURL(url); err != nil {
			log.Fatalf("Failed to extract from URL: %v", err)
		}
	} else {
		if verbose {
			log.Printf("Extracting from file: %s", input)
		}
		if err := extractor.ExtractFromFile(input); err != nil {
			log.Fatalf("Failed to extract from file: %v", err)
		}
	}

	if verbose {
		log.Printf("Found %d services, %d resources, %d operations", 
			len(extractor.services), len(extractor.resources), len(extractor.operations))
		
		// Count methods
		totalMethods := 0
		for _, methods := range extractor.serviceMethods {
			totalMethods += len(methods)
		}
		log.Printf("Mapped %d methods to services", totalMethods)
	}

	// Write to output file
	file, err := os.Create(output)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	
	if enhanced {
		// Write enhanced format
		extractor.WriteEnhancedWordlist(writer, patterns)
	} else {
		// Write simple format (not implemented in this version, but could be added)
		fmt.Fprintf(writer, "# Generated wordlist from API documentation\n")
		services := make([]string, 0, len(extractor.services))
		for service := range extractor.services {
			services = append(services, service)
		}
		sort.Strings(services)
		for _, service := range services {
			fmt.Fprintf(writer, "%s\n", service)
		}
	}

	writer.Flush()
	fmt.Printf("Wordlist saved to %s\n", output)
}