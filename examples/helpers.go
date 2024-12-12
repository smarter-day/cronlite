package examples

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// InitializeRedisClient initializes and returns a Redis client.
// It connects to the specified Redis address with default options.
func InitializeRedisClient(addr string) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: addr, // Redis server address
		// You can add more options here, such as Password and DB
	})

	// Ping the Redis server to ensure connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("Failed to connect to Redis at %s: %v\n", addr, err)
		os.Exit(1)
	}

	return client
}

// SetupSignalHandler sets up OS signal capturing to allow graceful shutdown.
// It listens for SIGINT and SIGTERM signals and cancels the provided context when received.
func SetupSignalHandler(cancelFunc context.CancelFunc) {
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)

	// Notify the channel on SIGINT and SIGTERM
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start a goroutine to listen for signals
	go func() {
		sig := <-sigChan
		fmt.Printf("Received signal: %s. Initiating shutdown...\n", sig)
		cancelFunc()
	}()
}
