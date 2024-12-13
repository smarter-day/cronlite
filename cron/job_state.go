package cron

import "time"

type CronJobStatus string

// CronJobState represents the state of a cron job at a given point in time.
// It includes information about the job's execution status, schedule, and metadata.
type CronJobState struct {
	// RunningBy indicates the identifier of the worker currently executing the job.
	RunningBy string `json:"running_by"`

	// Status represents the current status of the cron job, such as running, failed, or succeeded.
	Status CronJobStatus `json:"status"`

	// Iterations counts the number of times the job has been executed.
	Iterations int `json:"iterations"`

	// Spec is the cron expression that defines the job's schedule.
	Spec string `json:"spec"`

	// Data holds additional metadata or context information related to the job's execution.
	Data map[string]interface{} `json:"data"`

	// LastRun records the timestamp of the last time the job was executed.
	LastRun time.Time `json:"last_run"`

	// NextRun indicates the next scheduled execution time for the job.
	NextRun time.Time `json:"next_run"`

	// CreatedAt is the timestamp when the job state was initially created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the timestamp of the last update to the job state.
	UpdatedAt time.Time `json:"updated_at"`
}

// AddData adds key-value pairs to the Data map of the CronJobState.
// It accepts a variadic number of interface{} arguments, where each pair
// of arguments represents a key and its corresponding value.
//
// Parameters:
//
//	data: A variadic number of interface{} arguments. Each pair of arguments
//	      should consist of a string key followed by its associated value.
//
// Returns:
//
//	*CronJobState: The updated CronJobState instance with the new data added.
func (cjs *CronJobState) AddData(data ...interface{}) *CronJobState {
	for i := 0; i < len(data); i++ {
		if i%2 == 0 {
			cjs.Data[data[i].(string)] = data[i+1]
		}
	}
	cjs.UpdatedAt = time.Now()
	return cjs
}

// RemoveData removes specified keys from the Data map of the CronJobState.
// It updates the UpdatedAt timestamp to the current time after removing the keys.
//
// Parameters:
//
//	keys: A variadic number of string arguments representing the keys to be removed
//	      from the Data map.
//
// Returns:
//
//	*CronJobState: The updated CronJobState instance with the specified keys removed.
func (cjs *CronJobState) RemoveData(keys ...string) *CronJobState {
	for _, key := range keys {
		delete(cjs.Data, key)
	}
	cjs.UpdatedAt = time.Now()
	return cjs
}
