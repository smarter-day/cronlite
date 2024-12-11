#!/bin/bash

go install go.uber.org/mock/mockgen@latest

export PATH=$PATH:$(go env GOPATH)/bin

mockgen -source=./locker.go \
        -destination=./mocks/locker.go \
        -package=mocks

mockgen -source=./cron.go \
        -destination=./mocks/cron.go \
        -package=mocks

mockgen -source=./logger.go \
        -destination=./mocks/logger.go \
        -package=mocks

mockgen -package=mocks -destination=mocks/redis.go github.com/redis/go-redis/v9 Cmdable
