package main

import (
	"context"
	"cronlite/cron"
	"cronlite/helpers"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/smarter-day/logger"
	"github.com/spf13/cobra"
	"os"
	"time"
)

func main() {
	// Create a root context that will be canceled on program termination or timeout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal capturing to handle graceful shutdown
	helpers.SetupTerminationSignalHandler(cancel)
	log := logger.Log(ctx)

	// Define the root command using Cobra
	var rootCmd = &cobra.Command{
		Use:   "cronlite-example",
		Short: "A CLI tool to manage cron jobs using cronlite",
		Long:  `An example application demonstrating how to use cronlite with Cobra for scheduling cron jobs in Go.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Retrieve the job name from the flag
			jobName, err := cmd.Flags().GetString("name")
			if err != nil {
				fmt.Printf("Error retrieving 'name' flag: %v\n", err)
				os.Exit(1)
			}

			// Validate that the job name is provided
			if jobName == "" {
				fmt.Println("Error: --name flag is required.")
				err := cmd.Usage()
				if err != nil {
					return
				}
				os.Exit(1)
			}

			// Initialize the Redis client
			redisClient := helpers.InitializeRedisClient(&redis.Options{
				Addr: "localhost:6379",
			})
			defer func() {
				if err := redisClient.Close(); err != nil {
					fmt.Printf("Error closing Redis client: %v\n", err)
				}
			}()

			// Initialize the logger

			if err != nil {
				fmt.Printf("Failed to initialize logger: %v\n", err)
				return
			}

			// Define the cron job function
			jobFunction := func(ctx context.Context, job cron.ICronJob) error {
				log.Info("Executing cron job: Performing a scheduled task.")

				// Simulate a task taking some time
				time.Sleep(8 * time.Second)

				log.Info("CronJob job completed successfully.")
				return nil // Return nil to indicate success
			}

			// Define the cron job options
			jobOptions := cron.CronJobOptions{
				Redis:       redisClient,     // Redis client for state management and locking
				Name:        jobName,         // Unique name for the cron job
				Spec:        "*/5 * * * * *", // CronJob schedule: every 5 seconds
				ExecuteFunc: jobFunction,     // The job function to execute
			}

			// Create a new cron job instance
			cronJob, err := cron.NewCronJob(jobOptions)
			if err != nil {
				log.WithValues("error", err).
					Error("Failed to create cron job.")
				return
			}

			// Start the cron job
			if err := cronJob.Start(ctx); err != nil {
				log.WithValues("error", err).
					Error("Failed to start cron job.")
				return
			}

			log.Info("CronJob job started successfully.")

			// Create a context with a timeout of 1 minute
			timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 1*time.Minute)
			defer timeoutCancel()

			// Wait for either the parent context to be canceled or the timeout to occur
			select {
			case <-timeoutCtx.Done():
				if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
					log.Info("Timeout reached. Initiating shutdown...")
					cancel() // Cancel the parent context to initiate shutdown
				}
			case <-ctx.Done():
				// Context was canceled externally (e.g., via OS signal)
			}

			// Stop the cron job gracefully
			if err := cronJob.Stop(ctx); err != nil {
				log.WithValues("error", err).
					Error("Failed to stop cron job gracefully.")
			} else {
				log.Info("CronJob job stopped gracefully.")
			}
		},
	}

	// Define the --name flag as a required string flag
	rootCmd.Flags().StringP("name", "n", "", "Unique name for the cron job (required)")
	err := rootCmd.MarkFlagRequired("name")
	if err != nil {
		return
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Error("Error executing command")
		os.Exit(1)
	}
}
