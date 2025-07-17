# SPDX-License-Identifier: GPL-2.0-only

# Detect OS
OS := $(shell uname -s)

# Variables
OUTPUT := ../internal/ebpf
LIBBPF_HEADERS := /usr/include/bpf
ARCH := $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')

# Kernel feature detection (only run on Linux)
ifeq ($(OS),Linux)
    KERNEL_FEATURES := $(shell ./scripts/detect_kernel_features.sh env)
    BTF_SUPPORT := $(shell echo "$(KERNEL_FEATURES)" | grep BTF_SUPPORT | cut -d= -f2)
    CORE_SUPPORT := $(shell echo "$(KERNEL_FEATURES)" | grep CORE_SUPPORT | cut -d= -f2)
    BPF_FEATURES := $(shell echo "$(KERNEL_FEATURES)" | grep BPF_FEATURES | cut -d= -f2)
    KERNEL_VERSION := $(shell echo "$(KERNEL_FEATURES)" | grep KERNEL_VERSION | cut -d= -f2)
else
    # Default to full support for Docker builds
    BTF_SUPPORT := native
    CORE_SUPPORT := full
    BPF_FEATURES := ringbuf,tracepoint,kprobe,uprobe
    KERNEL_VERSION := unknown
endif

# Docker image for non-Linux builds
DOCKER_IMAGE := ebpf-build
# Mount the parent directory to allow access to ../internal/ebpf
DOCKER_RUN := docker run --rm -v $(shell cd .. && pwd):/workspace -w /workspace/ebpf $(DOCKER_IMAGE)

# Use Docker on non-Linux platforms, native tools on Linux
ifneq ($(OS),Linux)
	CLANG := $(DOCKER_RUN) clang
	LLC := $(DOCKER_RUN) llc
	BPFTOOL := $(DOCKER_RUN) /usr/local/bin/bpftool
	MKDIR := mkdir -p
	CD := cd
else
	CLANG ?= clang
	LLC ?= llc
	BPFTOOL ?= bpftool
	MKDIR := mkdir -p
	CD := cd
endif

# Compiler flags
INCLUDES := -I$(LIBBPF_HEADERS) -I./include

# Base compilation flags
BASE_CFLAGS := -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(ARCH) -D__BPF_TRACING__

# CO-RE-specific compilation flags for portability across kernel versions
ifeq ($(CORE_SUPPORT),full)
    CORE_CFLAGS := -mcpu=v1 -fno-stack-protector -fno-ident -fdebug-types-section \
                   -mllvm -bpf-expand-memcpy-in-order -DCORE_SUPPORT_FULL
else ifeq ($(CORE_SUPPORT),partial)
    CORE_CFLAGS := -mcpu=v1 -fno-stack-protector -DCORE_SUPPORT_PARTIAL
else
    CORE_CFLAGS := -DCORE_SUPPORT_NONE
endif

# Feature-specific flags
FEATURE_CFLAGS := 
ifeq ($(findstring ringbuf,$(BPF_FEATURES)),ringbuf)
    FEATURE_CFLAGS += -DHAS_RINGBUF_SUPPORT
endif
ifeq ($(findstring tracepoint,$(BPF_FEATURES)),tracepoint)
    FEATURE_CFLAGS += -DHAS_TRACEPOINT_SUPPORT
endif
ifeq ($(findstring kprobe,$(BPF_FEATURES)),kprobe)
    FEATURE_CFLAGS += -DHAS_KPROBE_SUPPORT
endif
ifeq ($(findstring uprobe,$(BPF_FEATURES)),uprobe)
    FEATURE_CFLAGS += -DHAS_UPROBE_SUPPORT
endif

# Combined flags
CFLAGS := $(BASE_CFLAGS) $(CORE_CFLAGS) $(FEATURE_CFLAGS)

# eBPF programs
BPF_PROGS := execsnoop process_collector

# Build all eBPF programs
.PHONY: all
all: $(BPF_PROGS)

# Create output directory
$(OUTPUT):
	@mkdir -p $(OUTPUT)

# Check if Docker image exists (non-Linux platforms)
.PHONY: docker-build-check
docker-build-check:
ifneq ($(OS),Linux)
	@if ! docker images | grep -q "^$(DOCKER_IMAGE) "; then \
		echo "Docker image $(DOCKER_IMAGE) not found. Building..."; \
		$(MAKE) docker-build; \
	fi
endif

# Generic rule to build eBPF programs
define build_bpf
$(1): $(OUTPUT) src/$(1).bpf.c docker-build-check
	@echo "Building eBPF program: $(1) (CO-RE: $(CORE_SUPPORT), BTF: $(BTF_SUPPORT))"
ifneq ($(OS),Linux)
	@echo "Using Docker for non-Linux build"
	@$(MKDIR) $(OUTPUT)/$(1)
	@$(DOCKER_RUN) sh -c "clang $(CFLAGS) $(INCLUDES) -c src/$(1).bpf.c -o $(OUTPUT)/$(1)/$(1).bpf.o && \
		echo 'Generating BTF information...' && \
		(pahole -J $(OUTPUT)/$(1)/$(1).bpf.o 2>/dev/null || \
		 llvm-objcopy --only-section=.BTF --set-section-flags .BTF=alloc,readonly $(OUTPUT)/$(1)/$(1).bpf.o $(OUTPUT)/$(1)/$(1).bpf.o.btf) && \
		echo 'Validating BTF...' && \
		bpftool btf dump file $(OUTPUT)/$(1)/$(1).bpf.o > /dev/null && \
		echo 'Generating skeleton...' && \
		cd $(OUTPUT)/$(1) && \
		bpftool gen skeleton $(1).bpf.o > $(1).bpf.h"
	@echo "✓ Successfully built $(1) with CO-RE support"
else
	@$(MKDIR) $(OUTPUT)/$(1)
	@echo "Compiling $(1) with CO-RE flags: $(CFLAGS)"
	$(CLANG) $(CFLAGS) $(INCLUDES) -c src/$(1).bpf.c -o $(OUTPUT)/$(1)/$(1).bpf.o
	@echo "Generating BTF information for $(1)..."
ifeq ($(BTF_SUPPORT),native)
	@# BTF is already generated during compilation with -fdebug-types-section
	@echo "✓ BTF generated during compilation"
else ifeq ($(BTF_SUPPORT),pahole)
	@# Generate BTF using pahole
	@pahole -J $(OUTPUT)/$(1)/$(1).bpf.o 2>/dev/null || \
		(echo "Warning: pahole failed, trying llvm-objcopy" && \
		 llvm-objcopy --only-section=.BTF --set-section-flags .BTF=alloc,readonly $(OUTPUT)/$(1)/$(1).bpf.o $(OUTPUT)/$(1)/$(1).bpf.o.btf)
else
	@echo "Warning: No BTF support available"
endif
	@echo "Validating BTF for $(1)..."
	@$(BPFTOOL) btf dump file $(OUTPUT)/$(1)/$(1).bpf.o > /dev/null 2>&1 || echo "Warning: BTF validation failed"
	@echo "Generating skeleton for $(1)..."
	$(CD) $(OUTPUT)/$(1) && $(BPFTOOL) gen skeleton $(1).bpf.o > $(1).bpf.h
	@echo "✓ Successfully built $(1) with CO-RE support"
endif
endef

# Generate build rules for each program
$(foreach prog,$(BPF_PROGS),$(eval $(call build_bpf,$(prog))))


# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning eBPF build artifacts"
	@rm -rf $(OUTPUT)/*/

# Build Docker image for non-Linux platforms
.PHONY: docker-build
docker-build:
	@if docker images | grep -q "^$(DOCKER_IMAGE) "; then \
		echo "Docker image $(DOCKER_IMAGE) already exists. Use 'make docker-rebuild' to force rebuild"; \
	else \
		echo "Building Docker image for eBPF development"; \
		docker build -t $(DOCKER_IMAGE) -f Dockerfile.builder .; \
	fi

# Force rebuild Docker image
.PHONY: docker-rebuild
docker-rebuild:
	@echo "Force rebuilding Docker image for eBPF development"
	docker build -t $(DOCKER_IMAGE) -f Dockerfile.builder .

# Verify BPF programs (Linux only)
.PHONY: verify
verify: all
ifneq ($(OS),Linux)
	@echo "Verify target is only available on Linux"
	@exit 1
else
	@echo "Verifying eBPF programs"
	@for prog in $(BPF_PROGS); do \
		echo "Verifying $$prog..."; \
		$(BPFTOOL) prog load $(OUTPUT)/$$prog/$$prog.bpf.o /sys/fs/bpf/$$prog_test || exit 1; \
		$(BPFTOOL) prog detach /sys/fs/bpf/$$prog_test || true; \
		rm -f /sys/fs/bpf/$$prog_test; \
	done
	@echo "All programs verified successfully"
endif

# Show kernel capabilities
.PHONY: capabilities
capabilities:
ifeq ($(OS),Linux)
	@./scripts/detect_kernel_features.sh summary
else
	@echo "Kernel capability detection is only available on Linux"
	@echo "Docker builds assume full CO-RE support"
endif

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all         - Build all eBPF programs (.o and .h files)"
ifneq ($(OS),Linux)
	@echo "  docker-build - Build Docker image if it doesn't exist"
	@echo "  docker-rebuild - Force rebuild Docker image"
endif
	@echo "  clean       - Remove build artifacts"
	@echo "  verify      - Verify eBPF programs can be loaded (Linux only)"
	@echo "  capabilities - Show kernel CO-RE capabilities (Linux only)"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Programs: $(BPF_PROGS)"
ifeq ($(OS),Linux)
	@echo ""
	@echo "Current kernel capabilities:"
	@echo "  CO-RE Support: $(CORE_SUPPORT)"
	@echo "  BTF Support: $(BTF_SUPPORT)"
	@echo "  Kernel Version: $(KERNEL_VERSION)"
	@echo "  BPF Features: $(BPF_FEATURES)"
endif
ifneq ($(OS),Linux)
	@echo ""
	@echo "Note: Running on non-Linux platform - builds will use Docker automatically"
	@echo "Docker image will be built on first use if it doesn't exist"
endif
