#!/usr/bin/env bash
# This script regenerates the type definitions from eBPF headers

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Run the main generation script
exec "${PROJECT_ROOT}/scripts/generate-ebpf-types.sh"
