# **CronLite: Lightweight Distributed Cron Job Manager**

CronLite is a lightweight, Redis-backed distributed cron job manager for handling scheduled tasks across multiple workers. 
Designed for simplicity and efficiency, it provides robust locking, state management, and easy-to-use APIs for building resilient cron-based applications.

---

## **Features**
- **Distributed Execution:** Ensure tasks are executed only once across multiple workers.
- **Flexible Scheduling:** Define schedules using standard cron expressions.
- **Redis-Based State Management:** Persist job states and retrieve them efficiently.
- **Locking Mechanism:** Prevent duplicate execution using a robust Redis lock.
- **Extensible Architecture:** Integrate custom job logic and logging with ease.
- **Job Recency Listing:** Quickly fetch jobs sorted by recency for monitoring.

---

## **Installation**

To use CronLite in your project:

1. Install the package using `go get`:
   ```bash
   go get github.com/smarter-day/cronlite
   ```

2. Import CronLite into your project:
   ```go
   import "github.com/smarter-day/cronlite"
   ```

---

## **Getting Started**

### **1. Initialize a Cron Job**
```go
package main

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"cronlite"
)

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	options := cronlite.CronJobOptions{
		Redis: redisClient,
		Name:  "example-job",
		Spec:  "*/5 * * * * *", // Run every 5 seconds
		Job: func(ctx context.Context, job *cronlite.CronJob) error {
			fmt.Println("Executing job!")
			return nil
		},
	}

	cronJob, err := cronlite.NewCronJob(options)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize cron job: %v", err))
	}

	ctx := context.Background()
	go cronJob.Start(ctx)

	// Stop the job after 1 minute
	select {
	case <-time.After(1 * time.Minute):
		cronJob.Stop()
	}
}
```

---

### **2. Fetch All Jobs Sorted by Recency**
```go
package main

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"cronlite"
)

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	ctx := context.Background()
	jobs, err := cronlite.ListJobsByRecency(ctx, redisClient, 10) // Fetch top 10 jobs
	if err != nil {
		panic(fmt.Sprintf("Failed to fetch jobs: %v", err))
	}

	fmt.Println("Jobs sorted by recency:")
	for i, job := range jobs {
		fmt.Printf("%d. %s\n", i+1, job)
	}
}
```

---

## **Configuration**
### Redis
CronLite relies on Redis for job state persistence and locking. Ensure Redis is installed and running:
```bash
docker run -d -p 6379:6379 redis
```

### Cron Expressions
Define job schedules using standard cron expressions. For example:
- `*/5 * * * * *` – Every 5 seconds
- `0 0 * * *` – Daily at midnight

---

## **API Reference**

### **CronJobOptions**
| Field       | Type                                      | Description                                  |
|-------------|-------------------------------------------|----------------------------------------------|
| `Redis`     | `redis.Cmdable`                          | Redis client for state and locking.         |
| `Name`      | `string`                                 | Unique name for the job.                    |
| `Spec`      | `string`                                 | Cron expression for scheduling.             |
| `Job`       | `func(ctx context.Context, job *CronJob) error` | Function executed on schedule.         |
| `Logger`    | `ILogger`                                | Custom logger (optional).                   |
| `Locker`    | `ILocker`                                | Custom locker (optional).                   |

---

## **CLI**
A simple command-line tool to list jobs by recency:

1. Build the CLI tool:
   ```bash
   go build -o jobs-cli cmd/jobs-cli.go
   ```

2. Run the tool with a limit:
   ```bash
   ./jobs-cli 10
   ```

---

## **Testing**
Run the tests using:
```bash
go test ./...
```

Ensure Redis is running before running tests.

---

## **Contributing**
We welcome contributions to CronLite! Please fork the repository and submit a pull request for review.

### **Steps to Contribute**
1. Fork the repository.
2. Create a new branch for your feature or bugfix:
   ```bash
   git checkout -b feature/new-feature
   ```
3. Write tests and ensure all existing tests pass.
4. Submit a pull request.

---

## **License**
CronLite is open-source and licensed under the [MIT License](LICENSE).

---

## **Acknowledgments**
CronLite is inspired by the need for a simple and reliable distributed cron job system. Thanks to the contributors and the Go community for their support!