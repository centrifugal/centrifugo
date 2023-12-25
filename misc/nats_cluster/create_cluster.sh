#!/bin/bash

# Function to check if a NATS node is up
wait_for_nats() {
  local port=$1
  while ! nc -z localhost "$port" > /dev/null 2>&1; do
    echo "Waiting for NATS node on port $port to start..."
    sleep 1
  done
  echo "NATS node on port $port is available."
}

# Array to hold PIDs of NATS server processes
declare -a nats_pids

# Function to start NATS cluster nodes
start_nats_cluster() {
  local nodes=$1
  local base_port=4222
  local cluster_port_base=5222
  local log_dir="./nats_logs"
  local cluster_nodes=()

  # Create log directory, clean previous
  rm -rf "$log_dir"
  mkdir -p "$log_dir"

  # Start NATS nodes
  for ((i=0; i<nodes; i++)); do
    local port=$((base_port + i))
    local cluster_port=$((cluster_port_base + i))
    local log_file="$log_dir/nats_node_${port}.log"

    # Start a NATS server
    nats-server --port "$port" --config server.conf --cluster nats://localhost:"$cluster_port" \
                --routes nats://localhost:"$cluster_port_base" \
                > "$log_file" 2>&1 &
    nats_pids[i]=$!
    cluster_nodes+=("localhost:$port")
  done

  # Wait for all nodes to become available
  for port in "${cluster_nodes[@]}"; do
    wait_for_nats "${port##*:}"
  done
}

# Function to shut down NATS cluster nodes
shutdown_nats_cluster() {
  echo "Shutting down NATS cluster nodes..."
  for pid in "${nats_pids[@]}"; do
    kill "$pid"
  done
}

# Trap Ctrl+C (SIGINT) to call shutdown function
trap shutdown_nats_cluster SIGINT

# Check if the number of nodes is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <number_of_nodes>"
  exit 1
fi

# Start the NATS cluster
start_nats_cluster "$1"

# Wait until a keyboard interrupt (Ctrl+C) is detected
echo "NATS cluster is running. Press Ctrl+C to stop."
wait
