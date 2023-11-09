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

# Function to start Redis cluster nodes
start_redis_cluster() {
  local masters=$1
  local replicas=$2
  local base_port=7000
  local cluster_dir="./cluster_data"
  local total_nodes=$((masters * (replicas + 1)))
  local redis_nodes=()

  # Create cluster directory, clean previous.
  rm -r "$cluster_dir"
  mkdir -p "$cluster_dir"

  # Start master nodes
  for ((i=0; i<masters; i++)); do
    local port=$((base_port + i))
    local node_dir="$cluster_dir/$port"
    mkdir -p "$node_dir"
    pushd "$node_dir" > /dev/null

    # Start a Redis server with cluster enabled and no replicas
    redis-server --port "$port" --cluster-enabled yes \
                 --cluster-config-file "nodes.conf" \
                 --cluster-node-timeout 5000 --save "" --appendonly no \
                 --appendfilename "appendonly.aof" \
                 --dbfilename "dump.rdb" \
                 --logfile "redis.log" &
    redis_pids[i]=$!
    popd > /dev/null
    redis_nodes+=("127.0.0.1:$port")
  done

  # Start replica nodes
  for ((i=0; i<masters * replicas; i++)); do
    local port=$((base_port + masters + i))
    local node_dir="$cluster_dir/$port"
    mkdir -p "$node_dir"
    pushd "$node_dir" > /dev/null

    # Start a Redis server with cluster enabled and no replicas
    redis-server --port "$port" --cluster-enabled yes \
                 --cluster-config-file "nodes.conf" \
                 --cluster-node-timeout 5000 --save "" --appendonly no \
                 --appendfilename "appendonly.aof" \
                 --dbfilename "dump.rdb" \
                 --logfile "redis.log" &
    local pid=$!
    redis_pids+=("$pid")
    popd > /dev/null
    redis_nodes+=("127.0.0.1:$port")
  done

  # Wait for all nodes to become available
  for port in "${redis_nodes[@]}"; do
    wait_for_redis "${port##*:}"
  done

  # Create the cluster with replicas
  echo "Creating Redis cluster with the following nodes: ${redis_nodes[*]}"
  yes "yes" | redis-cli --cluster create "${redis_nodes[@]}" --cluster-replicas "$replicas"
}

# Function to shut down Redis cluster nodes
shutdown_redis_cluster() {
  echo "Shutting down Redis cluster nodes..."
  for pid in "${redis_pids[@]}"; do
    kill "$pid"
  done
}

# Trap Ctrl+C (SIGINT) to call shutdown function
trap shutdown_redis_cluster SIGINT

# Check if the number of masters and replicas is provided
if [ -z "$1" ] || [ -z "$2" ]; then
  echo "Usage: $0 <number_of_masters> <number_of_replicas_per_master>"
  exit 1
fi

# Start the Redis cluster
start_redis_cluster "$1" "$2"

# Wait until a keyboard interrupt (Ctrl+C) is detected
echo "Redis cluster is running. Press Ctrl+C to stop."
wait

# The script will wait at the above line until the user presses Ctrl+C, which will trigger the shutdown function
