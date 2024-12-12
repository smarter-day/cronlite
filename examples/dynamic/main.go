package main

import (
	"context"
	"cronlite/cron"
	"cronlite/helpers"
	"cronlite/logger"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"math/rand"
	"os"
	"time"
)

func main() {
	// Create a root context that will be canceled on program termination or timeout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal capturing to handle graceful shutdown
	helpers.SetupSignalHandler(cancel)

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
			appLogger, err := logger.NewDynamicLogger("logrus", "debug")
			if err != nil {
				fmt.Printf("Failed to initialize logger: %v\n", err)
				return
			}

			// Define the cron job function
			jobFunction := func(ctx context.Context, job *cron.Job) error {
				appLogger.Info(ctx, "Executing cron job: Performing a scheduled task.", nil)

				// Simulate a task taking some time
				time.Sleep(8 * time.Second)

				appLogger.Info(ctx, "Cron job completed successfully.", nil)
				return nil // Return nil to indicate success
			}

			// Define the BeforeExecute hook
			afterExecute := func(ctx context.Context, job cron.IJob, err error) error {
				state, err := job.GetState().Get(ctx, false)
				if err != nil {
					appLogger.Error(ctx, "Failed to retrieve job state.", map[string]interface{}{"error": err})
					return err
				}

				if state.Iterations == 0 {
					// First execution, proceed as normal
					return nil
				}

				// Generate random integer between 1 and 50
				randomInt := rand.Intn(3) + 1
				newSpec := fmt.Sprintf("* */%d * * * * *", randomInt)

				// Update the cron job's Spec
				state.Data["spec"] = newSpec
				err = job.GetState().Save(ctx, state)
				if err != nil {
					return err
				} // Save the updated state
				if err != nil {
					appLogger.Error(ctx, "Failed to update cron spec.", map[string]interface{}{"error": err})
					return err
				}

				appLogger.Info(ctx, "Changed cron spec by random integer seconds.", map[string]interface{}{"randomInt": randomInt})

				return nil
			}

			// Define the cron job options
			jobOptions := cron.JobOptions{
				Redis:        redisClient,       // Redis client for state management and locking
				Name:         jobName,           // Unique name for the cron job
				Spec:         "*/5 * * * * * *", // Cron schedule: every 5 seconds
				Job:          jobFunction,       // The job function to execute
				Logger:       appLogger,         // Logger for logging job activities
				AfterExecute: afterExecute,      // BeforeExecute hook for conditional execution
			}

			// Create a new cron job instance
			cronJob, err := cron.NewJob(jobOptions)
			if err != nil {
				appLogger.Error(ctx, "Failed to create cron job.", map[string]interface{}{"error": err})
				return
			}

			// Start the cron job
			if err := cronJob.Start(ctx); err != nil {
				appLogger.Error(ctx, "Failed to start cron job.", map[string]interface{}{"error": err})
				return
			}

			appLogger.Info(ctx, "Cron job started successfully.", nil)

			// Create a context with a timeout of 1 minute
			timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 24*time.Hour)
			defer timeoutCancel()

			// Wait for either the parent context to be canceled or the timeout to occur
			select {
			case <-timeoutCtx.Done():
				if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
					appLogger.Info(ctx, "Timeout reached. Initiating shutdown...", nil)
					cancel() // Cancel the parent context to initiate shutdown
				}
			case <-ctx.Done():
				// Context was canceled externally (e.g., via OS signal)
			}

			// Stop the cron job gracefully
			if err := cronJob.Stop(ctx); err != nil {
				appLogger.Error(ctx, "Failed to stop cron job gracefully.", map[string]interface{}{"error": err})
			} else {
				appLogger.Info(ctx, "Cron job stopped gracefully.", nil)
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
		fmt.Printf("Error executing command: %v\n", err)
		os.Exit(1)
	}
}
