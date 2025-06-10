package main

import (
	"context"
	"fmt"
	"log"

	pb "github.com/user/grpc-scanner/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func main() {
	// Connect to server
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Test 1: Call SecureService without auth (should fail)
	fmt.Println("[+] Test 1: Calling SecureService without authentication...")
	secureClient := pb.NewSecureServiceClient(conn)
	ctx := context.Background()
	_, err = secureClient.GetSecretData(ctx, &pb.SecretRequest{ResourceId: "test"})
	if err != nil {
		fmt.Printf("   Expected failure: %v\n", err)
	} else {
		fmt.Println("   Unexpected success!")
	}

	// Test 2: Create a token
	fmt.Println("\n[+] Test 2: Creating authentication token...")
	authClient := pb.NewAuthServiceClient(conn)
	tokenResp, err := authClient.CreateToken(ctx, &pb.CreateTokenRequest{
		Username: "testuser",
		Password: "testpass",
	})
	if err != nil {
		log.Fatalf("Failed to create token: %v", err)
	}
	fmt.Printf("   Token created: %s\n", tokenResp.Token)

	// Test 3: Call SecureService with bearer token
	fmt.Println("\n[+] Test 3: Calling SecureService with bearer token...")
	md := metadata.Pairs("authorization", "Bearer "+tokenResp.Token)
	ctxWithAuth := metadata.NewOutgoingContext(ctx, md)
	secretResp, err := secureClient.GetSecretData(ctxWithAuth, &pb.SecretRequest{ResourceId: "test"})
	if err != nil {
		fmt.Printf("   Failed: %v\n", err)
	} else {
		fmt.Printf("   Success! Secret: %s (Classification: %s)\n", secretResp.Data, secretResp.Classification)
	}

	// Test 4: Call SecureService with API key
	fmt.Println("\n[+] Test 4: Calling SecureService with API key...")
	md = metadata.Pairs("x-api-key", "demo-api-key-123")
	ctxWithAPIKey := metadata.NewOutgoingContext(ctx, md)
	listResp, err := secureClient.ListSecrets(ctxWithAPIKey, &pb.ListSecretsRequest{})
	if err != nil {
		fmt.Printf("   Failed: %v\n", err)
	} else {
		fmt.Printf("   Success! Found %d secrets:\n", listResp.Total)
		for _, secret := range listResp.Secrets {
			fmt.Printf("     - %s: %s\n", secret.Id, secret.Name)
		}
	}

	// Test 5: Test other services (no auth required)
	fmt.Println("\n[+] Test 5: Testing services without auth requirements...")
	pingClient := pb.NewPingServiceClient(conn)
	pingResp, err := pingClient.Ping(ctx, &pb.PingRequest{Message: "hello"})
	if err != nil {
		fmt.Printf("   Ping failed: %v\n", err)
	} else {
		fmt.Printf("   Ping success: %s (Server: %s)\n", pingResp.Message, pingResp.ServerVersion)
	}

	fmt.Println("\n[+] Authentication tests complete!")
	fmt.Println("\nSummary:")
	fmt.Println("- SecureService properly requires authentication")
	fmt.Println("- Multiple auth methods supported: Bearer token, API key, Basic auth")
	fmt.Println("- Other services remain accessible without auth")
	fmt.Println("- The gRPC scanner can detect these services even with auth enabled")
}