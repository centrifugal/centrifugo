#!/bin/bash

# Function to wait for Redis node to become available
wait_for_redis() {
  local port=$1
  while ! redis-cli -p "$port" ping > /dev/null 2>&1; do
    echo "Waiting for Redis node on port $port to start..."
    sleep 1
  done
  echo "Redis node on port $port is available."
}

# Array to hold PIDs of Redis server processes
declare -a redis_pids

# Function to start Redis nodes
start_redis_instances() {
  local masters=$1
  local base_port=6000
  local redis_dir="./redis_data"
  local total_nodes=$((masters * (0 + 1)))
  local redis_nodes=()

  # Create cluster directory, clean previous.
  rm -r "$redis_dir"
  mkdir -p "$redis_dir"

  # Start master nodes
  for ((i=0; i<masters; i++)); do
    local port=$((base_port + i))
    local node_dir="$redis_dir/$port"
    mkdir -p "$node_dir"
    pushd "$node_dir" > /dev/null

    redis-server --port "$port" \
                 --save "" --appendonly no \
                 --appendfilename "appendonly.aof" \
                 --dbfilename "dump.rdb" \
                 --logfile "redis.log" &
    redis_pids[i]=$!
    popd > /dev/null
    redis_nodes+=("127.0.0.1:$port")
  done

  # Wait for all nodes to become available
  for port in "${redis_nodes[@]}"; do
    wait_for_redis "${port##*:}"
  done
}

# Function to shut down Redis nodes
shutdown_redis_instances() {
  echo "Shutting down Redis nodes..."
  for pid in "${redis_pids[@]}"; do
    kill "$pid"
  done
}

# Trap Ctrl+C (SIGINT) to call shutdown function
trap shutdown_redis_instances SIGINT

# Check if the number of masters and replicas is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <number_of_masters>"
  exit 1
fi

# Start Redis instances
start_redis_instances "$1"

# Wait until a keyboard interrupt (Ctrl+C) is detected
echo "Redis instances are running. Press Ctrl+C to stop."
wait

# The script will wait at the above line until the user presses Ctrl+C, which will trigger the shutdown function
