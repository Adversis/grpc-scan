package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	pb "github.com/user/grpc-scanner/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// HelloServer implements the HelloService
type helloServer struct {
	pb.UnimplementedHelloServiceServer
}

// SayHello implements the HelloService SayHello RPC method
func (s *helloServer) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloResponse, error) {
	log.Printf("Received: %v", req.GetName())
	return &pb.HelloResponse{Message: "Hello " + req.GetName()}, nil
}

// StreamHello implements the HelloService StreamHello RPC method
func (s *helloServer) StreamHello(req *pb.HelloRequest, stream pb.HelloService_StreamHelloServer) error {
	log.Printf("Stream request from: %v", req.GetName())

	// Send 5 messages with a delay between them
	for i := 0; i < 5; i++ {
		message := fmt.Sprintf("Hello %s! Message %d", req.GetName(), i+1)
		if err := stream.Send(&pb.HelloResponse{Message: message}); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// UserServer implements the UserService
type userServer struct {
	pb.UnimplementedUserServiceServer
	// Simulated user database
	users map[string]*pb.ProfileResponse
	// Simulated active sessions (username -> token)
	sessions map[string]string
}

// newUserServer creates a new UserServer instance with sample data
func newUserServer() *userServer {
	// Create a server with some sample users
	users := map[string]*pb.ProfileResponse{
		"user1": {
			UserId:    "user1",
			Username:  "john_doe",
			Email:     "john@example.com",
			FullName:  "John Doe",
			CreatedAt: "2023-01-01T00:00:00Z",
		},
		"user2": {
			UserId:    "user2",
			Username:  "jane_smith",
			Email:     "jane@example.com",
			FullName:  "Jane Smith",
			CreatedAt: "2023-02-15T00:00:00Z",
		},
	}
	return &userServer{
		users:    users,
		sessions: make(map[string]string),
	}
}

// Login implements the UserService Login RPC method
func (s *userServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	log.Printf("Login request from: %v", req.GetUsername())

	// Simple authentication logic (in a real app, you'd verify against a database)
	if req.GetUsername() == "john_doe" && req.GetPassword() == "password123" {
		// Generate a simple token (in a real app, you'd use JWT or similar)
		token := fmt.Sprintf("token_%d", time.Now().Unix())
		s.sessions[req.GetUsername()] = token

		return &pb.LoginResponse{
			Success: true,
			Token:   token,
		}, nil
	} else if req.GetUsername() == "jane_smith" && req.GetPassword() == "password456" {
		token := fmt.Sprintf("token_%d", time.Now().Unix())
		s.sessions[req.GetUsername()] = token

		return &pb.LoginResponse{
			Success: true,
			Token:   token,
		}, nil
	}

	return &pb.LoginResponse{
		Success:      false,
		ErrorMessage: "Invalid username or password",
	}, nil
}

// Register implements the UserService Register RPC method
func (s *userServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	log.Printf("Register request for: %v", req.GetUsername())

	// Check if username already exists
	for _, user := range s.users {
		if user.Username == req.GetUsername() {
			return &pb.RegisterResponse{
				Success:      false,
				ErrorMessage: "Username already exists",
			}, nil
		}
	}

	// Create a new user (in a real app, you'd store in a database)
	userId := fmt.Sprintf("user%d", len(s.users)+1)
	s.users[userId] = &pb.ProfileResponse{
		UserId:    userId,
		Username:  req.GetUsername(),
		Email:     req.GetEmail(),
		FullName:  req.GetFullName(),
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	return &pb.RegisterResponse{
		Success: true,
		UserId:  userId,
	}, nil
}

// GetProfile implements the UserService GetProfile RPC method
func (s *userServer) GetProfile(ctx context.Context, req *pb.ProfileRequest) (*pb.ProfileResponse, error) {
	log.Printf("GetProfile request for user ID: %v", req.GetUserId())

	// Look up the user
	if user, exists := s.users[req.GetUserId()]; exists {
		return user, nil
	}

	// In a real app, you might return a gRPC error code
	return &pb.ProfileResponse{
		UserId:   req.GetUserId(),
		Username: "unknown",
	}, nil
}

// ProductServer implements the ProductService
type productServer struct {
	pb.UnimplementedProductServiceServer
	// Simulated product database
	products map[string]*pb.ProductResponse
}

// newProductServer creates a new ProductServer instance with sample data
func newProductServer() *productServer {
	// Create a server with some sample products
	products := map[string]*pb.ProductResponse{
		"prod1": {
			ProductId:   "prod1",
			Name:        "Smartphone",
			Description: "Latest model smartphone with high-resolution camera",
			Price:       799.99,
			Inventory:   50,
			Categories:  []string{"electronics", "phones"},
		},
		"prod2": {
			ProductId:   "prod2",
			Name:        "Laptop",
			Description: "Powerful laptop for work and gaming",
			Price:       1299.99,
			Inventory:   30,
			Categories:  []string{"electronics", "computers"},
		},
		"prod3": {
			ProductId:   "prod3",
			Name:        "Wireless Headphones",
			Description: "Noise-cancelling wireless headphones",
			Price:       199.99,
			Inventory:   100,
			Categories:  []string{"electronics", "audio"},
		},
	}
	return &productServer{
		products: products,
	}
}

// GetProduct implements the ProductService GetProduct RPC method
func (s *productServer) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.ProductResponse, error) {
	log.Printf("GetProduct request for product ID: %v", req.GetProductId())

	// Look up the product
	if product, exists := s.products[req.GetProductId()]; exists {
		return product, nil
	}

	// In a real app, you might return a gRPC error code
	return &pb.ProductResponse{
		ProductId:   req.GetProductId(),
		Name:        "Unknown Product",
		Description: "Product not found",
	}, nil
}

// ListProducts implements the ProductService ListProducts RPC method
func (s *productServer) ListProducts(ctx context.Context, req *pb.ListProductsRequest) (*pb.ListProductsResponse, error) {
	log.Printf("ListProducts request with category: %v", req.GetCategory())

	var filteredProducts []*pb.ProductResponse

	// Apply filters
	for _, product := range s.products {
		// Filter by category if specified
		if req.GetCategory() != "" {
			categoryMatch := false
			for _, category := range product.Categories {
				if category == req.GetCategory() {
					categoryMatch = true
					break
				}
			}
			if !categoryMatch {
				continue
			}
		}

		// Filter by price range if specified
		if req.GetMinPrice() > 0 && product.Price < req.GetMinPrice() {
			continue
		}
		if req.GetMaxPrice() > 0 && product.Price > req.GetMaxPrice() {
			continue
		}

		filteredProducts = append(filteredProducts, product)
	}

	// Simple pagination (in a real app, you'd implement this more efficiently)
	page := req.GetPage()
	if page <= 0 {
		page = 1
	}
	pageSize := req.GetPageSize()
	if pageSize <= 0 {
		pageSize = 10
	}

	totalCount := len(filteredProducts)
	totalPages := (totalCount + int(pageSize) - 1) / int(pageSize)

	startIdx := (int(page) - 1) * int(pageSize)
	endIdx := startIdx + int(pageSize)

	if startIdx >= totalCount {
		// Return empty result for out-of-range pages
		return &pb.ListProductsResponse{
			Products:   []*pb.ProductResponse{},
			TotalCount: int32(totalCount),
			Page:       page,
			TotalPages: int32(totalPages),
		}, nil
	}

	if endIdx > totalCount {
		endIdx = totalCount
	}

	return &pb.ListProductsResponse{
		Products:   filteredProducts[startIdx:endIdx],
		TotalCount: int32(totalCount),
		Page:       page,
		TotalPages: int32(totalPages),
	}, nil
}

// PingServer implements the PingService
type pingServer struct {
	pb.UnimplementedPingServiceServer
	// Server version info
	version string
}

// newPingServer creates a new PingServer instance
func newPingServer() *pingServer {
	return &pingServer{
		version: "v1.0.0",
	}
}

// Ping implements the PingService Ping RPC method
func (s *pingServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	log.Printf("Ping request with message: %v", req.GetMessage())

	return &pb.PingResponse{
		Message:       "pong: " + req.GetMessage(),
		Timestamp:     time.Now().Format(time.RFC3339),
		ServerVersion: s.version,
	}, nil
}

// AuthService implements authentication-related RPCs
type authService struct {
	pb.UnimplementedAuthServiceServer
	// Valid API keys
	apiKeys map[string]bool
	// Valid tokens (token -> username)
	tokens map[string]string
}

// newAuthService creates a new AuthService instance
func newAuthService() *authService {
	return &authService{
		apiKeys: map[string]bool{
			"demo-api-key-123":  true,
			"test-api-key-456":  true,
			"admin-api-key-789": true,
		},
		tokens: make(map[string]string),
	}
}

// ValidateAPIKey implements API key validation
func (s *authService) ValidateAPIKey(ctx context.Context, req *pb.ValidateAPIKeyRequest) (*pb.ValidateAPIKeyResponse, error) {
	log.Printf("ValidateAPIKey request for key: %v", req.GetApiKey())
	
	if s.apiKeys[req.GetApiKey()] {
		return &pb.ValidateAPIKeyResponse{
			Valid: true,
		}, nil
	}
	
	return &pb.ValidateAPIKeyResponse{
		Valid: false,
		Error: "Invalid API key",
	}, nil
}

// CreateToken creates a new authentication token
func (s *authService) CreateToken(ctx context.Context, req *pb.CreateTokenRequest) (*pb.CreateTokenResponse, error) {
	log.Printf("CreateToken request for user: %v", req.GetUsername())
	
	// Simple validation (in real app, check password)
	if req.GetUsername() == "" || req.GetPassword() == "" {
		return &pb.CreateTokenResponse{
			Success: false,
			Error:   "Username and password required",
		}, status.Error(codes.InvalidArgument, "missing credentials")
	}
	
	// Generate token
	token := fmt.Sprintf("bearer_%s_%d", req.GetUsername(), time.Now().Unix())
	s.tokens[token] = req.GetUsername()
	
	return &pb.CreateTokenResponse{
		Success: true,
		Token:   token,
		ExpiresIn: 3600, // 1 hour
	}, nil
}

// ValidateToken validates a bearer token
func (s *authService) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	log.Printf("ValidateToken request")
	
	if username, ok := s.tokens[req.GetToken()]; ok {
		return &pb.ValidateTokenResponse{
			Valid:    true,
			Username: username,
		}, nil
	}
	
	return &pb.ValidateTokenResponse{
		Valid: false,
		Error: "Invalid or expired token",
	}, nil
}

// SecureService implements a service that requires authentication
type secureService struct {
	pb.UnimplementedSecureServiceServer
}

// GetSecretData implements a method that requires authentication
func (s *secureService) GetSecretData(ctx context.Context, req *pb.SecretRequest) (*pb.SecretResponse, error) {
	// This method will be protected by the auth interceptor
	log.Printf("GetSecretData request for: %v", req.GetResourceId())
	
	return &pb.SecretResponse{
		Data: fmt.Sprintf("Secret data for resource %s", req.GetResourceId()),
		Classification: "CONFIDENTIAL",
	}, nil
}

// ListSecrets implements a method that requires authentication
func (s *secureService) ListSecrets(ctx context.Context, req *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	log.Printf("ListSecrets request")
	
	// Return some dummy secrets
	secrets := []*pb.SecretInfo{
		{Id: "secret1", Name: "Database Password", CreatedAt: "2024-01-01"},
		{Id: "secret2", Name: "API Credentials", CreatedAt: "2024-01-02"},
		{Id: "secret3", Name: "Encryption Keys", CreatedAt: "2024-01-03"},
	}
	
	return &pb.ListSecretsResponse{
		Secrets: secrets,
		Total:   int32(len(secrets)),
	}, nil
}

// Auth interceptor functions
func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}
	
	// Check for Authorization header
	if vals := md.Get("authorization"); len(vals) > 0 {
		auth := vals[0]
		// Bearer token
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer "), nil
		}
		// Basic auth
		if strings.HasPrefix(auth, "Basic ") {
			encoded := strings.TrimPrefix(auth, "Basic ")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return "", status.Error(codes.Unauthenticated, "invalid basic auth encoding")
			}
			return string(decoded), nil
		}
	}
	
	// Check for API key
	if vals := md.Get("x-api-key"); len(vals) > 0 {
		return vals[0], nil
	}
	
	// Check for custom auth header
	if vals := md.Get("x-auth-token"); len(vals) > 0 {
		return vals[0], nil
	}
	
	return "", status.Error(codes.Unauthenticated, "missing authentication")
}

// unaryAuthInterceptor checks authentication for unary RPCs
func unaryAuthInterceptor(authSvc *authService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for certain methods
		if strings.Contains(info.FullMethod, "grpc.health") ||
			strings.Contains(info.FullMethod, "grpc.reflection") ||
			strings.Contains(info.FullMethod, "AuthService") ||
			strings.Contains(info.FullMethod, "PingService") ||
			strings.Contains(info.FullMethod, "HelloService") {
			return handler(ctx, req)
		}
		
		// Only require auth for SecureService
		if !strings.Contains(info.FullMethod, "SecureService") {
			return handler(ctx, req)
		}
		
		token, err := extractToken(ctx)
		if err != nil {
			return nil, err
		}
		
		// Validate token
		if strings.HasPrefix(token, "bearer_") {
			if _, ok := authSvc.tokens[token]; !ok {
				return nil, status.Error(codes.Unauthenticated, "invalid token")
			}
		} else if strings.Contains(token, ":") {
			// Basic auth format: username:password
			parts := strings.Split(token, ":")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, status.Error(codes.Unauthenticated, "invalid basic auth")
			}
		} else {
			// Assume it's an API key
			if !authSvc.apiKeys[token] {
				return nil, status.Error(codes.Unauthenticated, "invalid API key")
			}
		}
		
		return handler(ctx, req)
	}
}

func main() {
	port := flag.Int("port", 50051, "The server port")
	enableHealth := flag.Bool("health", true, "Enable health service")
	enableReflection := flag.Bool("reflection", false, "Enable reflection service")
	enableAuth := flag.Bool("auth", true, "Enable authentication")
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create auth service
	authSvc := newAuthService()

	// Create server with or without auth interceptor
	var s *grpc.Server
	if *enableAuth {
		s = grpc.NewServer(
			grpc.UnaryInterceptor(unaryAuthInterceptor(authSvc)),
		)
		log.Println("Authentication enabled")
	} else {
		s = grpc.NewServer()
	}

	// Register all services
	pb.RegisterHelloServiceServer(s, &helloServer{})
	pb.RegisterUserServiceServer(s, newUserServer())
	pb.RegisterProductServiceServer(s, newProductServer())
	pb.RegisterPingServiceServer(s, newPingServer())
	pb.RegisterAuthServiceServer(s, authSvc)
	pb.RegisterSecureServiceServer(s, &secureService{})

	// Register health service if enabled
	if *enableHealth {
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.HelloService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.UserService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.ProductService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.PingService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.AuthService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.SecureService", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(s, healthServer)
		log.Println("Health service registered")
	}

	// Register reflection service if enabled
	if *enableReflection {
		reflection.Register(s)
		log.Println("Reflection service registered")
	}

	log.Printf("Server listening on :%d", *port)
	log.Println("Available services:")
	log.Println("  - HelloService (no auth required)")
	log.Println("  - UserService (no auth required)")
	log.Println("  - ProductService (no auth required)")
	log.Println("  - PingService (no auth required)")
	log.Println("  - AuthService (for authentication)")
	log.Println("  - SecureService (requires auth: API key, Bearer token, or Basic auth)")
	if *enableAuth {
		log.Println("\nValid credentials:")
		log.Println("  API Keys: demo-api-key-123, test-api-key-456, admin-api-key-789")
		log.Println("  Basic Auth: any username:password")
		log.Println("  Bearer Token: use AuthService.CreateToken to generate")
	}
	
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
