package main

import (
	"context"
	"cronlite/cron"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
	"os"
	"strconv"
	"time"
)

func main() {
	var detailed bool

	var rootCmd = &cobra.Command{
		Use:   "go run examples/list/main.go [limit] [--detailed]",
		Short: "A CLI tool to list jobs by recency",
		Long:  `This CLI tool lists jobs sorted by their most recent updates. Use --detailed for more info.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Define Redis connection details
			redisAddr := "localhost:6379" // Change if Redis is running on a different host/port
			redisClient := redis.NewClient(&redis.Options{
				Addr: redisAddr,
			})

			limit, err := strconv.Atoi(args[0])
			if err != nil || limit <= 0 {
				return fmt.Errorf("limit must be a positive integer")
			}

			ctx := context.Background()

			if !detailed {
				jobNames, err := cron.ListJobsByRecency(ctx, redisClient, limit)
				if err != nil {
					return fmt.Errorf("error fetching jobs: %w", err)
				}

				fmt.Println("Jobs sorted by recency:")
				for i, jobName := range jobNames {
					fmt.Printf("%d. %s\n", i+1, jobName)
				}
			} else {
				jobsWithState, err := cron.ListJobsByRecencyWithState(ctx, redisClient, limit)
				if err != nil {
					return fmt.Errorf("error fetching jobs with state: %w", err)
				}

				table := tablewriter.NewWriter(os.Stdout)
				table.SetHeader([]string{"#", "Job Name", "Status", "Running By", "Last Run", "Next Run", "Iterations", "Created At", "Updated At"})

				formatRunningBy := func(s string) string {
					if len(s) <= 10 {
						return s
					}
					return s[:5] + "..." + s[len(s)-5:]
				}

				for i, job := range jobsWithState {
					// Format times
					lastRunStr := job.State.LastRun.Format(time.RFC3339)
					nextRunStr := job.State.NextRun.Format(time.RFC3339)
					createdAtStr := job.State.CreatedAt.Format(time.RFC3339)
					updatedAtStr := job.State.UpdatedAt.Format(time.RFC3339)

					table.Append([]string{
						strconv.Itoa(i + 1),
						job.JobName,
						string(job.State.Status),
						formatRunningBy(job.State.RunningBy),
						lastRunStr,
						nextRunStr,
						strconv.Itoa(job.State.Iterations),
						createdAtStr,
						updatedAtStr,
					})
				}

				table.Render()
			}

			return nil
		},
	}

	rootCmd.Flags().BoolVar(&detailed, "detailed", false, "Show detailed job states")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
