package main

import (
	"context"
	"cronlite/helpers"
	"errors"
	"github.com/redis/go-redis/v9"
	"math/rand"
	"os"
	"time"

	// Import the cronlite packages
	"cronlite/cron"
	"github.com/smarter-day/logger"

	// Import Cobra packages
	"github.com/spf13/cobra"
)

func main() {
	// Create a root context that will be canceled on program termination
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal capturing to handle graceful shutdown
	helpers.SetupTerminationSignalHandler(cancel)
	log := logger.Log(context.Background())

	// Define the root command using Cobra
	var rootCmd = &cobra.Command{
		Use:   "cronlite-example",
		Short: "A CLI tool to manage cron jobs using cronlite",
		Long:  `An example application demonstrating how to use cronlite with Cobra for scheduling cron jobs in Go.`,
		Run: func(cmd *cobra.Command, args []string) {

			// Retrieve the job name from the flag
			jobName, err := cmd.Flags().GetString("name")
			if err != nil {
				log.WithError(err).Error("Error retrieving 'name' flag")
				os.Exit(1)
			}

			// Validate that the job name is provided
			if jobName == "" {
				log.Error("Error: --name flag is required")
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
					log.WithError(err).Error("Error closing Redis client")
				}
			}()

			// Define the cron job function
			jobFunction := func(ctx context.Context, job cron.ICronJob) error {
				log.Info("Executing cron job: Performing a scheduled task")

				// Simulate a task taking some time
				randomSleep := time.Duration(rand.Intn(30)) * time.Second
				time.Sleep(randomSleep)

				randomError := rand.Intn(10) == 0
				if randomError {
					return errors.New("simulated error during job execution")
				}

				log.Info("Cron job completed successfully")
				return nil // Return nil to indicate success
			}

			// Define the cron job options
			jobOptions := cron.CronJobOptions{
				Redis:       redisClient,      // Redis client for state management and locking
				Name:        jobName,          // Unique name for the cron job
				Spec:        "*/10 * * * * *", // CronJob schedule: every 5 seconds
				ExecuteFunc: jobFunction,      // The job function to execute
			}

			// Create a new cron job instance
			cronJob, err := cron.NewCronJob(jobOptions)
			if err != nil {
				log.WithError(err).Error("Failed to create cron job")
				return
			}

			// Start the cron job in a separate goroutine
			if err := cronJob.Start(ctx); err != nil {
				log.WithError(err).Error("Failed to start cron job")
				return
			}

			log.Info("CronJob job started successfully")

			// Wait for the context to be canceled (e.g., via an OS signal)
			<-ctx.Done()

			// Stop the cron job gracefully
			if err := cronJob.Stop(ctx); err != nil {
				log.WithError(err).Error("Failed to stop cron job gracefully")
			} else {
				log.Info("CronJob job stopped gracefully")
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
