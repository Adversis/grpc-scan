package main

import (
	"context"
	"flag"
	"io"
	"log"
	"time"

	pb "github.com/user/grpc-scanner/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	target := flag.String("target", "localhost:50051", "The server address in the format host:port")
	flag.Parse()

	// Set up a connection to the server with insecure transport
	conn, err := grpc.Dial(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewHelloServiceClient(conn)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Call the SayHello unary RPC
	callUnaryRPC(ctx, c)

	// Call the StreamHello streaming RPC
	callStreamingRPC(ctx, c)
}

func callUnaryRPC(ctx context.Context, c pb.HelloServiceClient) {
	log.Println("Calling SayHello unary RPC...")
	resp, err := c.SayHello(ctx, &pb.HelloRequest{Name: "Client"})
	if err != nil {
		log.Fatalf("SayHello RPC failed: %v", err)
	}
	log.Printf("Response from server: %s", resp.GetMessage())
}

func callStreamingRPC(ctx context.Context, c pb.HelloServiceClient) {
	log.Println("Calling StreamHello streaming RPC...")
	stream, err := c.StreamHello(ctx, &pb.HelloRequest{Name: "Client"})
	if err != nil {
		log.Fatalf("StreamHello RPC failed: %v", err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			// End of stream
			break
		}
		if err != nil {
			log.Fatalf("Error while receiving stream: %v", err)
		}
		log.Printf("Stream response: %s", resp.GetMessage())
	}
}
