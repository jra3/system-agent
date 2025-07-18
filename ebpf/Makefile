# SPDX-License-Identifier: GPL-2.0-only

# Detect OS
OS := $(shell uname -s)

# Variables
OUTPUT := ../internal/ebpf
LIBBPF_HEADERS := /usr/include/bpf
ARCH := $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')

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
# Ensure vmlinux.h is available for native Linux builds
VMLINUX_H := ./include/vmlinux.h
# CO-RE-specific flags:
# -g: Enable BTF generation (debug info)
# -O2: Optimization level
# -Wall: Enable all warnings
# -target bpf: Target BPF architecture
# -fdebug-types-section: Improve BTF generation
# -fno-stack-protector: Disable stack protector (not supported in BPF)
CFLAGS := -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(ARCH) \
	-fdebug-types-section -fno-stack-protector

# eBPF programs
BPF_PROGS := execsnoop

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

# Generate vmlinux.h from running kernel BTF (Linux only)
$(VMLINUX_H):
ifeq ($(OS),Linux)
	@echo "Generating vmlinux.h from kernel BTF"
	@if [ -f /sys/kernel/btf/vmlinux ]; then \
		$(BPFTOOL) btf dump file /sys/kernel/btf/vmlinux format c > $(VMLINUX_H) && \
		echo "Generated vmlinux.h with $$(wc -l < $(VMLINUX_H)) lines"; \
	else \
		echo "Kernel BTF not available, using btfhub fallback"; \
		$(MAKE) vmlinux-btfhub; \
	fi
else
	@echo "vmlinux.h generation skipped on non-Linux platform"
endif

# Fallback: Download vmlinux.h from btfhub (for older kernels)
.PHONY: vmlinux-btfhub
vmlinux-btfhub:
	@echo "Downloading pre-generated vmlinux.h from btfhub"
	@KERNEL_VERSION=$$(uname -r) && \
	ARCH=$$(uname -m) && \
	case "$$ARCH" in \
		x86_64) BTF_ARCH="amd64" ;; \
		aarch64) BTF_ARCH="arm64" ;; \
		*) echo "Unsupported architecture: $$ARCH" && exit 1 ;; \
	esac && \
	BTF_URL="https://github.com/aquasecurity/btfhub-archive/raw/main/ubuntu/20.04/$$BTF_ARCH/5.8.0-63-generic.btf.tar.xz" && \
	echo "Downloading BTF from: $$BTF_URL" && \
	curl -sL "$$BTF_URL" -o /tmp/btf.tar.xz && \
	tar -xJf /tmp/btf.tar.xz -C /tmp && \
	$(BPFTOOL) btf dump file /tmp/5.8.0-63-generic.btf format c > $(VMLINUX_H) && \
	rm -f /tmp/btf.tar.xz /tmp/5.8.0-63-generic.btf && \
	echo "Generated vmlinux.h with $$(wc -l < $(VMLINUX_H)) lines"

# Generic rule to build eBPF programs
define build_bpf
$(1): $(OUTPUT) src/$(1).bpf.c $(VMLINUX_H) docker-build-check
	@echo "Building eBPF program: $(1)"
ifneq ($(OS),Linux)
	@echo "Using Docker for non-Linux build"
	@$(MKDIR) $(OUTPUT)/$(1)
	@$(DOCKER_RUN) sh -c "clang $(CFLAGS) $(INCLUDES) -c src/$(1).bpf.c -o $(OUTPUT)/$(1)/$(1).bpf.o && \
		echo 'Verifying BTF information for $(1)' && \
		bpftool btf dump file $(OUTPUT)/$(1)/$(1).bpf.o format raw > /dev/null && \
		cd $(OUTPUT)/$(1) && \
		bpftool gen skeleton $(1).bpf.o > $(1).bpf.h"
	@echo "Successfully built $(1)"
else
	@$(MKDIR) $(OUTPUT)/$(1)
	$(CLANG) $(CFLAGS) $(INCLUDES) -c src/$(1).bpf.c -o $(OUTPUT)/$(1)/$(1).bpf.o
	@echo "Verifying BTF information for $(1)"
	$(BPFTOOL) btf dump file $(OUTPUT)/$(1)/$(1).bpf.o format raw > /dev/null || \
		(echo "Warning: BTF verification failed for $(1)" && exit 1)
	@echo "Generating skeleton for $(1)"
	$(CD) $(OUTPUT)/$(1) && $(BPFTOOL) gen skeleton $(1).bpf.o > $(1).bpf.h
	@echo "Successfully built $(1)"
endif
endef

# Generate build rules for each program
$(foreach prog,$(BPF_PROGS),$(eval $(call build_bpf,$(prog))))


# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning eBPF build artifacts"
	@rm -rf $(OUTPUT)/*/
	@if [ -f $(VMLINUX_H) ]; then \
		echo "Removing generated vmlinux.h"; \
		rm -f $(VMLINUX_H); \
	fi

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

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all         - Build all eBPF programs (.o and .h files)"
ifneq ($(OS),Linux)
	@echo "  docker-build - Build Docker image if it doesn't exist"
	@echo "  docker-rebuild - Force rebuild Docker image"
endif
	@echo "  clean       - Remove build artifacts and generated vmlinux.h"
	@echo "  verify      - Verify eBPF programs can be loaded (Linux only)"
ifeq ($(OS),Linux)
	@echo "  vmlinux-btfhub - Force download vmlinux.h from btfhub (fallback)"
endif
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Programs: $(BPF_PROGS)"
ifeq ($(OS),Linux)
	@echo ""
	@echo "Note: vmlinux.h will be generated from kernel BTF if available"
	@echo "      Falls back to btfhub if kernel BTF not found"
endif
ifneq ($(OS),Linux)
	@echo ""
	@echo "Note: Running on non-Linux platform - builds will use Docker automatically"
	@echo "Docker image will be built on first use if it doesn't exist"
endif
