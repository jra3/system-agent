# SPDX-License-Identifier: GPL-2.0-only

# Detect OS
OS := $(shell uname -s)

# Variables
OUTPUT := ../internal/ebpf
LIBBPF_HEADERS := /usr/include/bpf
ARCH := $(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm64/')

# Docker image for Darwin builds
DOCKER_IMAGE := ebpf-build
# Mount the parent directory to allow access to ../internal/ebpf
DOCKER_RUN := docker run --rm -v $(shell cd .. && pwd):/workspace -w /workspace/ebpf $(DOCKER_IMAGE)

# Use Docker on Darwin, native tools on Linux
ifeq ($(OS),Darwin)
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
CFLAGS := -g -O2 -Wall -target bpf -D__TARGET_ARCH_$(ARCH)

# eBPF programs
BPF_PROGS := execsnoop

# Build all eBPF programs
.PHONY: all
all: $(BPF_PROGS)

# Create output directory
$(OUTPUT):
	@mkdir -p $(OUTPUT)

# Check if Docker image exists (Darwin only)
.PHONY: docker-build-check
docker-build-check:
ifeq ($(OS),Darwin)
	@if ! docker images | grep -q "^$(DOCKER_IMAGE) "; then \
		echo "Docker image $(DOCKER_IMAGE) not found. Building..."; \
		$(MAKE) docker-build; \
	fi
endif

# Generic rule to build eBPF programs
define build_bpf
$(1): $(OUTPUT) src/$(1).bpf.c docker-build-check
	@echo "Building eBPF program: $(1)"
ifeq ($(OS),Darwin)
	@echo "Using Docker for Darwin build"
	@$(MKDIR) $(OUTPUT)/$(1)
	@$(DOCKER_RUN) sh -c "clang $(CFLAGS) $(INCLUDES) -c src/$(1).bpf.c -o $(OUTPUT)/$(1)/$(1).bpf.o && \
		cd $(OUTPUT)/$(1) && \
		bpftool gen skeleton $(1).bpf.o > $(1).bpf.h"
	@echo "Successfully built $(1)"
else
	@$(MKDIR) $(OUTPUT)/$(1)
	$(CLANG) $(CFLAGS) $(INCLUDES) -c src/$(1).bpf.c -o $(OUTPUT)/$(1)/$(1).bpf.o
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

# Build Docker image for Darwin
.PHONY: docker-build
docker-build:
	@if docker images | grep -q "^$(DOCKER_IMAGE) "; then \
		echo "Docker image $(DOCKER_IMAGE) already exists. Use 'make docker-rebuild' to force rebuild"; \
	else \
		echo "Building Docker image for eBPF development"; \
		docker build -t $(DOCKER_IMAGE) .; \
	fi

# Force rebuild Docker image
.PHONY: docker-rebuild
docker-rebuild:
	@echo "Force rebuilding Docker image for eBPF development"
	docker build -t $(DOCKER_IMAGE) .

# Verify BPF programs (Linux only)
.PHONY: verify
verify: all
ifeq ($(OS),Darwin)
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
ifeq ($(OS),Darwin)
	@echo "  docker-build - Build Docker image if it doesn't exist"
	@echo "  docker-rebuild - Force rebuild Docker image"
endif
	@echo "  clean       - Remove build artifacts"
	@echo "  verify      - Verify eBPF programs can be loaded (Linux only)"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Programs: $(BPF_PROGS)"
ifeq ($(OS),Darwin)
	@echo ""
	@echo "Note: Running on Darwin - builds will use Docker automatically"
	@echo "Docker image will be built on first use if it doesn't exist"
endif
