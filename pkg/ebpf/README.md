# eBPF Go Bindings

This directory contains Go bindings and utilities for eBPF programs used by the Antimetal Agent.

## Structure

- Generated Go bindings from eBPF programs appear here

## Development Workflow

### Decentralized Generation Pattern

Place `//go:generate` directives in the userspace packages that will use the eBPF programs, rather than in a centralized location. This keeps the generation logic co-located with the code that uses it.

### Adding New eBPF Programs

1. **Create the eBPF source file**:
   ```
   ebpf/src/your_program.bpf.c
   ```

2. **Add go:generate directive** to the relevant userspace package:
   ```go
   // In pkg/performance/collectors/your_collector.go
   //go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang YourProgram ../../ebpf/src/your_program.bpf.c
   ```

3. **Generate Go bindings**:
   ```bash
   make generate-ebpf-bindings
   ```

### Adding New Type Definitions

1. **Create header file** with C struct definitions:
   ```
   ebpf/include/your_collector_types.h
   ```

2. **Generate Go types**:
   ```bash
   make generate-ebpf-types
   ```

3. **Generated files appear in**:
   ```
   pkg/performance/collectors/your_collector_types.go
   ```

### Common Locations for go:generate Directives

- `pkg/performance/collectors/` - For performance monitoring eBPF programs
- `pkg/ebpf/` - For general eBPF utilities

### Build Commands

- `make generate-ebpf-bindings` - Generate Go bindings from eBPF C code
- `make generate-ebpf-types` - Generate Go types from eBPF header files
- `make build-ebpf` - Build eBPF programs (uses Docker on non-Linux)

### Example: Adding a Memory Collector

1. **Create eBPF program**:
   ```c
   // ebpf/src/memory_collector.bpf.c
   #include "memory_collector_types.h"
   // ... eBPF program code
   ```

2. **Create types header**:
   ```c
   // ebpf/include/memory_collector_types.h
   struct memory_stats {
       uint64_t total_pages;
       uint64_t free_pages;
   };
   ```

3. **Add generation directive to collector**:
   ```go
   // pkg/performance/collectors/memory_collector.go
   //go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang MemoryCollector ../../ebpf/src/memory_collector.bpf.c
   
   type MemoryCollector struct {
       // ... collector fields
   }
   ```

4. **Generate bindings and types**:
   ```bash
   make generate-ebpf-bindings
   make generate-ebpf-types
   ```

This creates:
- `pkg/performance/collectors/memory_collector_bpf.go` - Generated eBPF bindings
- `pkg/performance/collectors/memory_collector_types.go` - Generated Go types

### Generated File Structure

```
pkg/performance/collectors/
├── memory_collector.go              // Your collector implementation with //go:generate
├── memory_collector_bpf.go          // Generated eBPF bindings
├── memory_collector_types.go        // Generated Go types from C headers
└── memory_collector_test.go         // Your tests
```

### Testing

eBPF programs should be tested through their Go userspace counterparts. The generated bindings provide the interface for loading and interacting with eBPF programs from Go code.

Test patterns:
- Mock eBPF map interactions
- Test program loading and unloading
- Validate data structure marshaling/unmarshaling
- Test error handling for eBPF operations

### Dependencies

- **github.com/cilium/ebpf** - eBPF library for Go
- **clang** - For compiling eBPF programs
- **libbpf** - eBPF library (for compilation)

## Decentralized Generation Benefits

This package uses a decentralized generation pattern where `//go:generate` directives are placed in the packages that use the eBPF programs.

### Why Decentralized?

1. **Co-location**: Generation logic is next to the code that uses it
2. **Maintainability**: Easier to track which programs are used where
3. **Modularity**: Each package manages its own eBPF dependencies
4. **Clarity**: Obvious relationship between userspace and kernel code