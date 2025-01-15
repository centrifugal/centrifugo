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

# Function to start Redis Sentinel setup
start_redis_sentinel() {
  local base_port=7000
  local sentinel_port=26379
  local data_dir="./sentinel_data"

  # Clean up previous setup
  rm -rf "$data_dir"
  mkdir -p "$data_dir"

  # Start the master node
  local master_port=$((base_port))
  local master_dir="$data_dir/master"
  mkdir -p "$master_dir"
  pushd "$master_dir" > /dev/null
  redis-server --port "$master_port" --save "" --appendonly no --dbfilename "dump.rdb" --logfile "redis.log" &
  local pid=$!
  redis_pids+=($pid)
  echo "Master started on port $master_port with PID $pid"
  popd > /dev/null

  # Start the replicas
  for i in 1 2; do
    local replica_port=$((base_port + i))
    local replica_dir="$data_dir/replica_$i"
    mkdir -p "$replica_dir"
    pushd "$replica_dir" > /dev/null
    redis-server --port "$replica_port" --slaveof 127.0.0.1 "$master_port" --save "" --appendonly no --dbfilename "dump.rdb" --logfile "redis.log" &
    pid=$!
    redis_pids+=($pid)
    echo "Replica $i started on port $replica_port with PID $pid"
    popd > /dev/null
  done

  # Wait for all nodes to become available
  wait_for_redis "$master_port"
  for i in 1 2; do
    wait_for_redis $((base_port + i))
  done

  # Start Redis Sentinel
  local sentinel_dir="$data_dir/sentinel"
  mkdir -p "$sentinel_dir"
  pushd "$sentinel_dir" > /dev/null

  cat <<EOF > sentinel.conf
port $sentinel_port
sentinel monitor mymaster 127.0.0.1 $master_port 1
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 10000
sentinel parallel-syncs mymaster 1
EOF

  redis-server sentinel.conf --sentinel &
  pid=$!
  redis_pids+=($pid)
  echo "Sentinel started on port $sentinel_port with PID $pid"
  popd > /dev/null
}

# Function to shut down Redis nodes and Sentinel
shutdown_redis_sentinel() {
  echo "Shutting down Redis nodes and Sentinel..."
  for pid in "${redis_pids[@]}"; do
    kill "$pid"
    echo "Stopped process with PID $pid"
  done
}

# Trap Ctrl+C (SIGINT) to call shutdown function
trap shutdown_redis_sentinel SIGINT

# Start the Redis Sentinel setup
start_redis_sentinel

# Wait until a keyboard interrupt (Ctrl+C) is detected
echo "Redis Sentinel setup is running. Press Ctrl+C to stop."
echo "All PIDs: ${redis_pids[*]}"
wait
