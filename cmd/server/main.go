package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	pb "github.com/user/grpc-scanner/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
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

func main() {
	port := flag.Int("port", 50051, "The server port")
	enableHealth := flag.Bool("health", true, "Enable health service")
	enableReflection := flag.Bool("reflection", false, "Enable reflection service")
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()

	// Register all services
	pb.RegisterHelloServiceServer(s, &helloServer{})
	pb.RegisterUserServiceServer(s, newUserServer())
	pb.RegisterProductServiceServer(s, newProductServer())
	pb.RegisterPingServiceServer(s, newPingServer())

	// Register health service if enabled
	if *enableHealth {
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.HelloService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.UserService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.ProductService", healthpb.HealthCheckResponse_SERVING)
		healthServer.SetServingStatus("proto.PingService", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(s, healthServer)
		log.Println("Health service registered")
	}

	// Register reflection service if enabled
	if *enableReflection {
		reflection.Register(s)
		log.Println("Reflection service registered")
	}

	log.Printf("Server listening on :%d", *port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
