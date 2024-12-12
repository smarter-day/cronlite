package cron

import "time"

const (
	// JobsList Stores the job names in a sorted set keyed by their most recent timestamp
	JobsList = "cronlite_jobs"

	// JobStateKeyFormat format for job state
	JobStateKeyFormat = "cronlite:job:state:%s"

	// StaleThreshold to consider a job stale if not updated.
	// When no updates (heartbeats) within 20s, consider not running.
	StaleThreshold = 20 * time.Second

	JobStatusRunning   JobStatus = "Running"
	JobStatusCancelled JobStatus = "Cancelled"
	JobStatusFailed    JobStatus = "Failed"
	JobStatusSuccess   JobStatus = "Success"
)
