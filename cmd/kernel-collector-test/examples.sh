#!/bin/bash
# Copyright Antimetal, Inc. All rights reserved.
#
# Use of this source code is governed by a source available license that can be found in the
# LICENSE file or at:
# https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

# Example usage scripts for kernel-collector-test

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="${SCRIPT_DIR}/kernel-collector-test"

# Check if binary exists
if [[ ! -f "${BINARY}" ]]; then
    echo "Error: kernel-collector-test binary not found!"
    echo "Please run: ${SCRIPT_DIR}/build.sh"
    exit 1
fi

echo "=== Kernel Collector Test Examples ==="
echo ""

# Function to run example with description
run_example() {
    local description="$1"
    local command="$2"
    
    echo "Example: ${description}"
    echo "Command: ${command}"
    echo "Press Enter to run (or Ctrl+C to skip)..."
    read -r
    
    echo "Running..."
    echo "---"
    eval "${command}"
    echo "---"
    echo ""
}

# Example 1: Basic point collection
run_example "Collect last 20 kernel messages" \
    "sudo ${BINARY} -limit 20"

# Example 2: Verbose point collection
run_example "Collect last 10 messages with verbose logging" \
    "sudo ${BINARY} -limit 10 -v"

# Example 3: Generate some kernel activity
run_example "Generate kernel messages by loading/unloading a module" \
    "sudo modprobe -r dummy 2>/dev/null; sudo modprobe dummy; sudo ${BINARY} -limit 5; sudo modprobe -r dummy"

# Example 4: Continuous collection (runs for 5 seconds)
run_example "Continuous collection for 5 seconds" \
    "timeout 5 sudo ${BINARY} -continuous || true"

# Example 5: Check capabilities
run_example "Show collector capabilities" \
    "${BINARY} -h"

echo "All examples complete!"