#!/bin/bash

while true; do
    echo "Running command: $@"
    "$@"
    echo "Command finished, sleeping for 10 seconds..."
    sleep 10
done
