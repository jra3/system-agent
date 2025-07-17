# eBPF Programs

This directory contains eBPF programs used by the Antimetal Agent. These programs are licensed under GPL-2.0-only, as required for eBPF programs that use GPL-only kernel helper functions.

## Licensing

All files in this directory are licensed under GPL-2.0-only. This is separate from the rest of the Antimetal Agent codebase, which uses PolyForm licensing.

## Structure

- `src/` - eBPF C source files
- `include/` - eBPF header files and type definitions
- `build/` - Built eBPF object files
- `LICENSE` - GPL-2.0-only license text

## Development Workflow

### Adding New eBPF Programs

1. **Create the eBPF source file**:
   ```
   ebpf/src/your_program.bpf.c
   ```

2. **Add go:generate directive** to the userspace package that will use the eBPF program:
   ```go
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

### Build Commands

- `make build-ebpf` - Build eBPF programs (uses Docker on non-Linux)
- `make build-ebpf-builder` - Build eBPF Docker image
- `make generate-ebpf-bindings` - Generate Go bindings from eBPF C code
- `make generate-ebpf-types` - Generate Go types from eBPF header files

### Generation Pattern

Place `//go:generate` directives in the userspace packages that will use the eBPF programs, rather than in a centralized location. This keeps the generation logic co-located with the code that uses it.

Common locations:
- `pkg/performance/collectors/` - For performance monitoring eBPF programs
- `pkg/ebpf/` - For general eBPF utilities

### Example Structure

```
ebpf/
├── src/
│   ├── memory_collector.bpf.c
│   └── cpu_collector.bpf.c
├── include/
│   ├── memory_collector_types.h
│   └── cpu_collector_types.h
└── build/
    ├── memory_collector.bpf.o
    └── cpu_collector.bpf.o
```

With corresponding Go files generated in:
```
pkg/performance/collectors/
├── memory_collector.go          // Contains //go:generate directive
├── memory_collector_bpf.go      // Generated bindings
├── memory_collector_types.go    // Generated types
├── cpu_collector.go             // Contains //go:generate directive
├── cpu_collector_bpf.go         // Generated bindings
└── cpu_collector_types.go       // Generated types
```

## Dependencies

- **clang** - For compiling eBPF programs
- **libbpf** - eBPF library
- **Docker** - For cross-platform builds (non-Linux systems)

## Testing

eBPF programs should be tested through their Go userspace counterparts. The generated bindings provide the interface for loading and interacting with eBPF programs from Go code.