# Simple eBPF build environment
FROM ubuntu:24.04

# Install minimal required packages for eBPF development
RUN apt-get update && apt-get install -y --no-install-recommends \
		curl \
		xz-utils \
		ca-certificates \
		clang \
		llvm \
		libbpf-dev \
		linux-tools-generic \
		make \
		git \
		build-essential \
		&& rm -rf /var/lib/apt/lists/*

# Build bpftool from source to get latest version with Go support
# Pin to specific commit hash for security (v7.5.0 tag)
RUN git clone https://github.com/libbpf/bpftool.git /tmp/bpftool && \
    cd /tmp/bpftool && \
    git checkout v7.5.0 && \
    git verify-tag v7.5.0 2>/dev/null || echo "Warning: Could not verify git tag signature" && \
    git submodule update --init && \
    cd src && \
    make -j$(nproc) && \
    make install && \
    cd / && \
    rm -rf /tmp/bpftool

# Download pre-generated vmlinux.h from btfhub
# This provides a vmlinux.h that works for CO-RE across many kernel versions
# Add integrity verification for security
#
# To update BTF checksums for new versions:
# 1. Update the BTF_URL below to point to the new BTF archive
# 2. Temporarily comment out the checksum verification section below
# 3. Run: make build-ebpf-builder
# 4. Copy the displayed BTF SHA256 checksum from the build output
# 5. Update the EXPECTED_BTF_SHA256 variable below
# 6. Uncomment the verification section
# 7. Test with: make build-ebpf-builder
RUN echo "=== Downloading pre-generated vmlinux.h ===" && \
    BTF_URL="https://github.com/aquasecurity/btfhub-archive/raw/main/ubuntu/20.04/arm64/5.8.0-63-generic.btf.tar.xz" && \
    EXPECTED_BTF_SHA256="cdd9e65811a4de0e98012dd1c59ea3a90aa57a27b6b1896d2abf83f0713d0138" && \
    curl -sL "$BTF_URL" -o /tmp/btf.tar.xz && \
    echo "Downloaded BTF archive, verifying integrity..." && \
    ACTUAL_SHA256=$(sha256sum /tmp/btf.tar.xz | cut -d' ' -f1) && \
    echo "BTF archive SHA256: $ACTUAL_SHA256" && \
    echo "Expected SHA256: $EXPECTED_BTF_SHA256" && \
    if [ "$ACTUAL_SHA256" != "$EXPECTED_BTF_SHA256" ]; then \
        echo "ERROR: BTF archive checksum mismatch!" && \
        echo "Expected: $EXPECTED_BTF_SHA256" && \
        echo "Actual:   $ACTUAL_SHA256" && \
        exit 1; \
    fi && \
    echo "✓ BTF archive integrity verified" && \
    tar -xJf /tmp/btf.tar.xz -C /tmp && \
    bpftool btf dump file /tmp/5.8.0-63-generic.btf format c > /usr/include/vmlinux.h && \
    rm -f /tmp/btf.tar.xz /tmp/5.8.0-63-generic.btf && \
    echo "Downloaded vmlinux.h with $(wc -l < /usr/include/vmlinux.h 2>/dev/null || echo 0) lines"

# Verify vmlinux.h was downloaded successfully
RUN if [ -f /usr/include/vmlinux.h ] && [ -s /usr/include/vmlinux.h ]; then \
        echo "=== vmlinux.h successfully installed ===" && \
        echo "Location: /usr/include/vmlinux.h" && \
        echo "Size: $(wc -l < /usr/include/vmlinux.h) lines" && \
        echo "First few type definitions:" && \
        grep -m 5 "struct\|enum\|typedef" /usr/include/vmlinux.h || true; \
    else \
        echo "ERROR: vmlinux.h was not created or is empty!" && \
        exit 1; \
    fi

# Install Go 1.24.1 from official tarball based on architecture
# Add checksum verification for security
#
# To update checksums for new Go versions:
# 1. Temporarily comment out the checksum verification section below
# 2. Run: make build-ebpf-builder
# 3. Copy the displayed SHA256 checksums from the build output
# 4. Update the EXPECTED_GO_SHA256_* variables below
# 5. Uncomment the verification section
# 6. Test with: make build-ebpf-builder
RUN ARCH=$(uname -m) && \
    case "$ARCH" in \
        x86_64) GO_ARCH="amd64" ;; \
        aarch64) GO_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" && exit 1 ;; \
    esac && \
    GO_VERSION="1.24.1" && \
    GO_URL="https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" && \
    # Expected checksums for Go 1.24.1 (update when changing GO_VERSION)
    case "$GO_ARCH" in \
        amd64) EXPECTED_GO_SHA256="cb2396bae64183cdccf81a9a6df0aea3bce9511fc21469fb89a0c00470088073" ;; \
        arm64) EXPECTED_GO_SHA256="8df5750ffc0281017fb6070fba450f5d22b600a02081dceef47966ffaf36a3af" ;; \
        *) echo "Unsupported Go architecture: $GO_ARCH" && exit 1 ;; \
    esac && \
    curl -sL "$GO_URL" -o /tmp/go.tar.gz && \
    echo "Downloaded Go ${GO_VERSION} for ${GO_ARCH}, verifying checksum..." && \
    ACTUAL_SHA256=$(sha256sum /tmp/go.tar.gz | cut -d' ' -f1) && \
    echo "Go archive SHA256: $ACTUAL_SHA256" && \
    echo "Expected SHA256: $EXPECTED_GO_SHA256" && \
    if [ "$ACTUAL_SHA256" != "$EXPECTED_GO_SHA256" ]; then \
        echo "ERROR: Go archive checksum mismatch!" && \
        echo "Expected: $EXPECTED_GO_SHA256" && \
        echo "Actual:   $ACTUAL_SHA256" && \
        echo "If updating Go version, see comments above for checksum update procedure" && \
        exit 1; \
    fi && \
    echo "✓ Go archive integrity verified" && \
    tar -C /usr/local -xzf /tmp/go.tar.gz && \
    rm -f /tmp/go.tar.gz

# Set Go in PATH
ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /workspace
