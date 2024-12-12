#!/bin/bash

go install go.uber.org/mock/mockgen@latest

export PATH=$PATH:$(go env GOPATH)/bin

mockgen -source=./locker/locker.go \
        -destination=./mocks/locker_mock.go \
        -package=mocks

mockgen -source=./cron/types.go \
        -destination=./mocks/cron_mock.go \
        -package=mocks

mockgen -source=./logger/logger.go \
        -destination=./mocks/logger_mock.go \
        -package=mocks

mockgen -package=mocks -destination=mocks/redis_interface.go github.com/redis/go-redis/v9 Cmdable,Pipeliner
