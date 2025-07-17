#!/bin/bash
# Copyright Antimetal, Inc. All rights reserved.
#
# Use of this source code is governed by a source available license that can be found in the
# LICENSE file or at:
# https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

set -euo pipefail

# Detect kernel CO-RE capabilities

detect_btf_support() {
    if [ -f /sys/kernel/btf/vmlinux ]; then
        echo "native"
    elif command -v pahole >/dev/null 2>&1; then
        echo "pahole"
    else
        echo "none"
    fi
}

detect_core_support() {
    BTF_SUPPORT=$(detect_btf_support)
    KERNEL_VERSION=$(uname -r | cut -d. -f1,2)
    
    case "$BTF_SUPPORT" in
        "native")
            echo "full"
            ;;
        "pahole")
            if [ "$(printf '%s\n' "4.18" "$KERNEL_VERSION" | sort -V | head -n1)" = "4.18" ]; then
                echo "partial"
            else
                echo "none"
            fi
            ;;
        *)
            echo "none"
            ;;
    esac
}

detect_bpf_features() {
    # Check for BPF program types support
    local features=""
    
    # Check for ring buffer support (kernel 5.8+)
    if [ -f /sys/kernel/debug/tracing/events/bpf/bpf_map_create/format ] && \
       grep -q "map_type.*BPF_MAP_TYPE_RINGBUF" /sys/kernel/debug/tracing/events/bpf/bpf_map_create/format 2>/dev/null; then
        features="$features,ringbuf"
    fi
    
    # Check for tracepoint support
    if [ -d /sys/kernel/debug/tracing/events ]; then
        features="$features,tracepoint"
    fi
    
    # Check for kprobe support
    if [ -f /sys/kernel/debug/tracing/kprobe_events ]; then
        features="$features,kprobe"
    fi
    
    # Check for uprobe support
    if [ -f /sys/kernel/debug/tracing/uprobe_events ]; then
        features="$features,uprobe"
    fi
    
    # Remove leading comma
    echo "${features#,}"
}

get_kernel_version() {
    uname -r
}

get_kernel_major_minor() {
    uname -r | cut -d. -f1,2
}

print_capability_summary() {
    echo "=== Kernel CO-RE Capability Detection ==="
    echo "Kernel Version: $(get_kernel_version)"
    echo "BTF Support: $(detect_btf_support)"
    echo "CO-RE Support: $(detect_core_support)"
    echo "BPF Features: $(detect_bpf_features)"
    echo "=========================================="
}

# Main detection logic
main() {
    case "${1:-summary}" in
        "btf")
            detect_btf_support
            ;;
        "core")
            detect_core_support
            ;;
        "features")
            detect_bpf_features
            ;;
        "version")
            get_kernel_version
            ;;
        "major-minor")
            get_kernel_major_minor
            ;;
        "summary")
            print_capability_summary
            ;;
        "env")
            # Output environment variables for Makefile
            echo "BTF_SUPPORT=$(detect_btf_support)"
            echo "CORE_SUPPORT=$(detect_core_support)"
            echo "BPF_FEATURES=$(detect_bpf_features)"
            echo "KERNEL_VERSION=$(get_kernel_version)"
            echo "KERNEL_MAJOR_MINOR=$(get_kernel_major_minor)"
            ;;
        *)
            echo "Usage: $0 [btf|core|features|version|major-minor|summary|env]"
            exit 1
            ;;
    esac
}

main "$@"