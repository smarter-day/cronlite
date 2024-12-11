package main

import (
	"context"
	"cronlite"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	logger, err := cronlite.NewDynamicLogger("logrus", "debug")
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		return
	}

	// Define the job function
	job := func(ctx context.Context, job *cronlite.CronJob) error {
		fmt.Println("Executing cron job: Performing a scheduled task.")
		time.Sleep(2 * time.Second)
		fmt.Println("Cron job completed.")
		return nil
	}

	// Cron job options
	options := cronlite.CronJobOptions{
		Redis:  redisClient,
		Name:   "example-cron-lock",
		Spec:   "*/5 * * * * * *", // Run every 5 seconds
		Job:    job,
		Logger: logger,
	}

	// Create a CronJob instance
	cronJob, err := cronlite.NewCronJob(options)
	if err != nil {
		fmt.Printf("Failed to create CronJob: %v\n", err)
		return
	}

	// Start the cron job with a context
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	cronJob.Start(ctx)

	// Wait for the context to timeout
	<-ctx.Done()

	// Stop the cron job
	cronJob.Stop()
	fmt.Println("Cron job stopped.")
}
