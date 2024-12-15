# **CronLite: Lightweight Distributed Cron Job Manager**

CronLite is a lightweight, Redis-backed distributed cron job manager designed to handle scheduled tasks across multiple workers. Built for simplicity and efficiency, CronLite offers robust locking, state management, and user-friendly APIs, making it ideal for developing resilient cron-based applications.

---

## **Features**

- **Distributed Execution:** Ensure tasks are executed only once across multiple workers.
- **Flexible Scheduling:** Define schedules using standard cron expressions.
- **Redis-Based State Management:** Persist job states and retrieve them efficiently.
- **Locking Mechanism:** Prevent duplicate executions using a robust Redis lock.
- **Extensible Architecture:** Integrate custom job logic and logging with ease.
- **Job Recency Listing:** Quickly fetch jobs sorted by recency for monitoring.

---

## **Installation**

To use CronLite in your project:

1. **Install the package using `go get`:**

   ```bash
   go get github.com/smarter-day/cronlite
   ```

2. **Import CronLite into your project:**

   ```go
   import "github.com/smarter-day/cronlite"
   ```

---

## **Getting Started**

### **Simple Example**

**1. Initialize a Cron Job**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/smarter-day/cronlite"
)

func main() {
    // Initialize Redis client
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // Define CronJob options
    options := cronlite.CronJobOptions{
        Redis: redisClient,
        Name:  "example-job",
        Spec:  "*/5 * * * * *", // Run every 5 seconds
        ExecuteFunc: func(ctx context.Context, job cronlite.ICronJob) error {
            fmt.Println("Executing job!")
            return nil
        },
    }

    // Create a new CronJob instance
    cronJob, err := cronlite.NewCronJob(options)
    if err != nil {
        panic(fmt.Sprintf("Failed to initialize cron job: %v", err))
    }

    // Start the CronJob in a separate goroutine
    ctx := context.Background()
    go cronJob.Start(ctx)

    // Stop the job after 1 minute
    select {
    case <-time.After(1 * time.Minute):
        cronJob.Stop(ctx)
    }
}
```

**2. Fetch All Jobs Sorted by Recency**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/smarter-day/cronlite"
)

func main() {
    // Initialize Redis client
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    ctx := context.Background()

    // Fetch top 10 jobs sorted by recency
    jobs, err := cronlite.ListJobsByRecency(ctx, redisClient, 10)
    if err != nil {
        panic(fmt.Sprintf("Failed to fetch jobs: %v", err))
    }

    fmt.Println("Jobs sorted by recency:")
    for i, job := range jobs {
        fmt.Printf("%d. %s\n", i+1, job)
    }
}
```

### **Example with `BeforeExecuteFunc` Hook**

The `BeforeExecuteFunc` hook allows you to execute separate logic before every execution of `ExecuteFunc`. This can be used to decide whether to run the cron job, modify the schedule, or perform any other preparatory tasks.

**Pre-execute Hook Example:**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/smarter-day/cronlite"
)

func main() {
    // Initialize Redis client
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // Define CronJob options with BeforeExecuteFunc
    options := cronlite.CronJobOptions{
        Redis: redisClient,
        Name:  "example-job",
        Spec:  "*/5 * * * * *", // Run every 5 seconds
        BeforeExecuteFunc: func(ctx context.Context, job cronlite.ICronJob) (bool, error) {
            // Decide whether to execute the job
            shouldExecute := checkSomeCondition()
            if !shouldExecute {
                fmt.Println("Skipping job execution based on pre-execute hook.")
            }
            return shouldExecute, nil
        },
        ExecuteFunc: func(ctx context.Context, job cronlite.ICronJob) error {
            fmt.Println("Executing job!")
            return nil
        },
    }

    // Create a new CronJob instance
    cronJob, err := cronlite.NewCronJob(options)
    if err != nil {
        panic(fmt.Sprintf("Failed to initialize cron job: %v", err))
    }

    // Start the CronJob in a separate goroutine
    ctx := context.Background()
    go cronJob.Start(ctx)

    // Stop the job after 1 minute
    select {
    case <-time.After(1 * time.Minute):
        cronJob.Stop(ctx)
    }
}

// checkSomeCondition determines whether the job should execute
func checkSomeCondition() bool {
    // Implement your condition logic here
    return true // or false based on the condition
}
```

### **Cron Job with `BeforeStartFunc` Hook**

The `BeforeStartFunc` hook is executed only once when the cron job system is initialized and started. This can be used to perform setup tasks, fetch additional resources, modify configurations, or decide whether the cron job should start.

**BeforeStartHook Example:**

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
    "github.com/smarter-day/cronlite"
)

func main() {
    // Initialize Redis client
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // Define CronJob options with BeforeStartFunc
    options := cronlite.CronJobOptions{
        Redis: redisClient,
        Name:  "example-job",
        Spec:  "*/5 * * * * *", // Run every 5 seconds
        BeforeStartFunc: func(ctx context.Context, job cronlite.ICronJob) (bool, error) {
            // Decide whether to start the job
            shouldStart := checkInitializationCondition()
            if !shouldStart {
                fmt.Println("Skipping job start based on pre-start hook.")
            }
            return shouldStart, nil
        },
        ExecuteFunc: func(ctx context.Context, job cronlite.ICronJob) error {
            fmt.Println("Executing job!")
            return nil
        },
    }

    // Create a new CronJob instance
    cronJob, err := cronlite.NewCronJob(options)
    if err != nil {
        panic(fmt.Sprintf("Failed to initialize cron job: %v", err))
    }

    // Start the CronJob in a separate goroutine
    ctx := context.Background()
    go cronJob.Start(ctx)

    // Stop the job after 1 minute
    select {
    case <-time.After(1 * time.Minute):
        cronJob.Stop(ctx)
    }
}

// checkInitializationCondition determines whether the job should start
func checkInitializationCondition() bool {
    // Implement your condition logic here
    return true // or false based on the condition
}
```

If `BeforeStartFunc` returns `false`, the job will not run and will automatically call its `Stop` method.

### **Cron Job with `AfterExecuteFunc` Hook**

Use `AfterExecuteFunc` to clean up things, or any other logic after successful or not `ExecuteFunc` execution.

### **Example with changing cron job spec dynamically**

Check out this example:

```bash
go run examples/dynamic/main.go --name dynamic
```

Very short code example:

```go
// Define BeforeStartFunc to change the job's Spec before starting
 beforeStart := func(ctx context.Context, job cronlite.ICronJob) (bool, error) {
     // New Spec: Run every 10 seconds
     newSpec := "*/10 * * * * *"
     state, err := job.GetState().Get(ctx, false)
     if err != nil {
         fmt.Printf("Failed to get job state: %v\n", err)
         return false, err
     }

     // Update the Spec
     state.Spec = newSpec
     err = job.GetState().Save(ctx, state)
     if err != nil {
         fmt.Printf("Failed to save updated job state: %v\n", err)
         return false, err
     }

     fmt.Printf("Updated job Spec to: %s\n", newSpec)
     return true, nil // Continue starting the job
 }

// Define CronJob options with BeforeStartFunc
options := cronlite.CronJobOptions{
   Redis:            redisClient,
   Name:             "hello-world-job",
   Spec:             "*/5 * * * * *", // Initial Spec: every 5 seconds
   ExecuteFunc:      jobFunction,
   BeforeStartFunc:  beforeStart,
}

// Create a new CronJob
cronJob, err := cronlite.NewCronJob(options)
if err != nil {
    panic(fmt.Sprintf("Failed to create cron job: %v", err))
}

// Start the CronJob
go cronJob.Start(ctx)
```

In upper example when cron system starts initially, it can theoretically get the new spec from somewhere else,
or adjust it due to some conditions.

What is important - the new spec will be saved to redis state, and the rest of 
workers will fetch it before the next execution.

It means, if you have rolling deployment, even before all the pods in your system 
will be replaced - existing cron instances will already be working with new spec.

---

## **Configuration**

### **Redis**

CronLite relies on Redis for job state persistence and locking. Ensure Redis is installed and running:

```bash
docker run -d -p 6379:6379 redis
```

### **Cron Expressions**

Define job schedules using standard cron expressions. Examples:

- `*/5 * * * * *` – Every 5 seconds
- `0 0 * * *` – Daily at midnight

---

## **API Reference**

### **CronJobOptions**

| Field               | Type                                                                             | Description                                                                                                |
|---------------------|----------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------|
| `Redis`             | `redis.Cmdable`                                                                  | Redis client for state management and locking.                                                             |
| `Name`              | `string`                                                                         | Unique name for the job.                                                                                   |
| `Spec`              | `string`                                                                         | Cron expression for scheduling.                                                                            |
| `ExecuteFunc`       | `func(ctx context.Context, job cronlite.ICronJob) error`                         | Function executed on schedule.                                                                             |
| `BeforeExecuteFunc` | `func(ctx context.Context, job cronlite.ICronJob) (bool, error)`(optional)       | Function executed before each job execution.                                                               |
| `BeforeStartFunc`   | `func(ctx context.Context, job cronlite.ICronJob) (bool, error)`(optional)       | Function executed before the job starts.                                                                   |
| `AfterExecuteFunc`  | `func(ctx context.Context, job cronlite.ICronJob, err error) error`(optional)    | Function executed after execution round. As third parameter it accepts error happened in execute function. |
| `Logger`            | `ILogger` (optional)                                                             | Custom logger interface for logging.                                                                       |
| `Locker`            | `ILocker` (optional)                                                             | Custom locker interface for lock management.                                                               |

---

## **CLI**

A simple command-line tool to list jobs by recency:

```bash
go run examples/simple/main.go --name test
```

**Usage Example:**

```bash
$ go run examples/list/main.go 10 --detailed
+---+----------------------------+------------------+---------+---------------+---------------------------+---------------------------+------------+---------------------------+---------------------------+
| # |        CRONJOB NAME        |       SPEC       | STATUS  |  RUNNING BY   |         LAST RUN          |         NEXT RUN          | ITERATIONS |        CREATED AT         |        UPDATED AT         |
+---+----------------------------+------------------+---------+---------------+---------------------------+---------------------------+------------+---------------------------+---------------------------+
| 1 | cronlite:job:state:test1   | */15 * * * * * | Success | 32da0...b32ee | 2024-12-15T11:05:01+02:00 | 2024-12-15T11:05:15+02:00 |       5750 | 2024-12-13T19:21:55+02:00 | 2024-12-15T11:05:03+02:00 |
| 2 | cronlite:job:state:dynamic | */34 * * * * * | Success | f0c2b...a1831 | 2024-12-15T05:01:52+02:00 | 2024-12-15T05:02:00+02:00 |       4514 | 2024-12-13T18:06:21+02:00 | 2024-12-15T05:02:00+02:00 |
+---+----------------------------+------------------+---------+---------------+---------------------------+---------------------------+------------+---------------------------+---------------------------+
```

*Note:* Ensure that the `examples/simple/main.go` file exists and is correctly implemented to support this CLI functionality.

---

## **Testing**

Run the tests using:

```bash
go test ./...
```

Ensure that you have all the necessary mock implementations and that Redis is running (or properly mocked) when executing tests.

---

## **Contributing**

We welcome contributions to CronLite! Follow these steps to contribute:

1. **Fork the repository.**
2. **Create a new branch for your feature or bugfix:**

   ```bash
   git checkout -b feature/new-feature
   ```

3. **Write tests and ensure all existing tests pass.**
4. **Submit a pull request for review.**

Please ensure that your code adheres to the project's coding standards and that it includes appropriate tests and documentation.

---

## **License**

CronLite is open-source and licensed under the [MIT License](LICENSE).

---

## **Acknowledgments**

CronLite was inspired by the need for a simple and reliable distributed cron job system. Thanks to the contributors and the Go community for their support!
