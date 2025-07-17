// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package ebpf

// This file contains go:generate directives for generating Go bindings from eBPF programs.
// The directives use CO-RE-specific flags to ensure proper compilation and BTF generation.

// CO-RE-specific flags for bpf2go:
// -cc clang: Use clang as the compiler
// -cflags: CO-RE compilation flags including BTF generation
// -type: Export specific types from the eBPF program
// -go-package: Specify the Go package name for generated code

// Process monitoring eBPF program with CO-RE support
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type process_event -go-package ebpf ProcessCollector ../../ebpf/src/process_collector.bpf.c

// Network monitoring eBPF program with CO-RE support
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type network_event -go-package ebpf NetworkCollector ../../ebpf/src/network_collector.bpf.c

// File system monitoring eBPF program with CO-RE support
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type fs_event -go-package ebpf FilesystemCollector ../../ebpf/src/filesystem_collector.bpf.c

// System call monitoring eBPF program with CO-RE support
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type syscall_event -go-package ebpf SyscallCollector ../../ebpf/src/syscall_collector.bpf.c

// Performance monitoring eBPF program with CO-RE support
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type perf_event -go-package ebpf PerformanceCollector ../../ebpf/src/performance_collector.bpf.c

// Note: The actual eBPF source files (.bpf.c) will be created in Phase 2 of the CO-RE implementation.
// For now, these directives are prepared but commented out to avoid build errors.
// Uncomment them as the corresponding .bpf.c files are created.

/*
// Uncomment these directives as the corresponding .bpf.c files are created:

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type process_event -go-package ebpf ProcessCollector ../../ebpf/src/process_collector.bpf.c

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type network_event -go-package ebpf NetworkCollector ../../ebpf/src/network_collector.bpf.c

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type fs_event -go-package ebpf FilesystemCollector ../../ebpf/src/filesystem_collector.bpf.c

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type syscall_event -go-package ebpf SyscallCollector ../../ebpf/src/syscall_collector.bpf.c

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -target bpf -mcpu=v1 -fdebug-types-section -fno-stack-protector -mllvm -bpf-expand-memcpy-in-order -I../../ebpf/include -DCORE_SUPPORT_FULL" -type perf_event -go-package ebpf PerformanceCollector ../../ebpf/src/performance_collector.bpf.c
*/