#!/bin/bash

go install go.uber.org/mock/mockgen@latest

export PATH=$PATH:$(go env GOPATH)/bin

# External
mockgen -package=mocks -destination=mocks/redis_interface.go github.com/redis/go-redis/v9 Cmdable,Pipeliner

# Local
mockgen -source=./locker/locker.go \
        -destination=./mocks/locker_mock.go \
        -package=mocks

mockgen -source=./cron/constants.go \
        -destination=./mocks/cron_constants_mock.go \
        -package=mocks

mockgen -source=./cron/helpers.go \
        -destination=./mocks/cron_helpers_mock.go \
        -package=mocks

mockgen -source=./cron/job.go \
        -destination=./mocks/cron_job_mock.go \
        -package=mocks

mockgen -source=./cron/state.go \
        -destination=./mocks/cron_state_mock.go \
        -package=mocks

mockgen -source=./cron/job_state.go \
        -destination=./mocks/cron_job_state_mock.go \
        -package=mocks

mockgen -source=./cron/worker.go \
        -destination=./mocks/cron_worker_mock.go \
        -package=mocks
