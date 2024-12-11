package main

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"os"
	"strconv"
)

// ListJobsByRecency fetches the list of jobs sorted by most recent updates
func ListJobsByRecency(ctx context.Context, redisClient *redis.Client, limit int) ([]string, error) {
	jobNames, err := redisClient.ZRevRange(ctx, "cronlite:jobs", 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs by recency: %w", err)
	}
	return jobNames, nil
}

func main() {
	// Define Redis connection details
	redisAddr := "localhost:6379" // Change if Redis is running on a different host/port
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// Parse command-line arguments
	args := os.Args[1:]
	if len(args) < 1 {
		fmt.Println("Usage: jobs-cli <limit>")
		os.Exit(1)
	}

	// Get the limit from the command-line argument
	limit, err := strconv.Atoi(args[0])
	if err != nil || limit <= 0 {
		fmt.Println("Error: Limit must be a positive integer.")
		os.Exit(1)
	}

	// Fetch and print the list of jobs sorted by recency
	ctx := context.Background()
	jobNames, err := ListJobsByRecency(ctx, redisClient, limit)
	if err != nil {
		fmt.Printf("Error fetching jobs: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Jobs sorted by recency:")
	for i, jobName := range jobNames {
		fmt.Printf("%d. %s\n", i+1, jobName)
	}
}
