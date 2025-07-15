#!/bin/bash
# Copyright Antimetal, Inc. All rights reserved.
#
# Use of this source code is governed by a source available license that can be found in the
# LICENSE file or at:
# https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BINARY_NAME="kernel-collector-test"

echo "Building kernel collector test harness..."

cd "${PROJECT_ROOT}"

# Build the binary
go build -o "${SCRIPT_DIR}/${BINARY_NAME}" "${SCRIPT_DIR}/main.go"

echo "Build complete: ${SCRIPT_DIR}/${BINARY_NAME}"

# Check if install was requested
if [[ "$1" == "install" ]]; then
    INSTALL_PATH="/usr/local/bin/${BINARY_NAME}"
    echo "Installing to ${INSTALL_PATH}..."
    sudo cp "${SCRIPT_DIR}/${BINARY_NAME}" "${INSTALL_PATH}"
    echo "Installation complete!"
    echo ""
    echo "You can now run: sudo ${BINARY_NAME}"
else
    echo ""
    echo "To install system-wide, run: $0 install"
    echo "To run locally: sudo ${SCRIPT_DIR}/${BINARY_NAME}"
fi