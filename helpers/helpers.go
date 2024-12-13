package helpers

import (
	"context"
	"cronlite/logger"
	"fmt"
	"github.com/redis/go-redis/v9"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// InitializeRedisClient creates and returns a new Redis client using the provided options.
// It pings the Redis server to ensure connectivity and exits the program if the connection fails.
//
// Parameters:
//
//	options - A pointer to redis.Options containing the configuration for the Redis client.
//
// Returns:
//
//	A pointer to a redis.Client that is connected to the Redis server.
func InitializeRedisClient(options *redis.Options) *redis.Client {
	client := redis.NewClient(options)

	// Ping the Redis server to ensure connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("Failed to connect to Redis at %s: %v\n", options.Addr, err)
		os.Exit(1)
	}

	return client
}

// SetupTerminationSignalHandler sets up a signal handler to gracefully handle OS signals
// such as SIGINT and SIGTERM. When a signal is received, it calls the provided
// cancel function to initiate a shutdown process.
//
// Parameters:
//
//	cancelFunc - A context.CancelFunc that is called to cancel the context
//	             and initiate the shutdown process when a signal is received.
func SetupTerminationSignalHandler(cancelFunc context.CancelFunc) {
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)

	// Notify the channel on SIGINT and SIGTERM
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)

	// Start a goroutine to listen for signals
	go func() {
		sig := <-sigChan
		logger.Log(context.Background()).WithValues("signal", sig).
			Info("Received termination signal. Initiating shutdown...")
		cancelFunc()
	}()
}
