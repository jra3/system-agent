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
		&& rm -rf /var/lib/apt/lists/*

# Download pre-generated vmlinux.h from btfhub
# This provides a vmlinux.h that works for CO-RE across many kernel versions
RUN echo "=== Downloading pre-generated vmlinux.h ===" && \
    # Use btfhub's vmlinux headers which are known to work well for CO-RE
    curl -sL https://github.com/aquasecurity/btfhub-archive/raw/main/ubuntu/24.04/arm64/6.8.0-49-generic.btf.tar.xz | \
        tar -xJ -O | \
        bpftool btf dump file /dev/stdin format c > /usr/include/vmlinux.h 2>/dev/null || \
    # Fallback to x86_64 if arm64 fails
    (echo "ARM64 BTF not found, trying x86_64..." && \
     curl -sL https://github.com/aquasecurity/btfhub-archive/raw/main/ubuntu/24.04/x86_64/6.8.0-49-generic.btf.tar.xz | \
        tar -xJ -O | \
        bpftool btf dump file /dev/stdin format c > /usr/include/vmlinux.h) || \
    # Final fallback: download from libbpf-bootstrap
    (echo "Falling back to libbpf-bootstrap vmlinux.h..." && \
     curl -sL https://raw.githubusercontent.com/libbpf/libbpf-bootstrap/master/vmlinux/vmlinux.h > /usr/include/vmlinux.h) && \
    echo "Downloaded vmlinux.h with $(wc -l < /usr/include/vmlinux.h 2>/dev/null || echo 0) lines"

# Make bpftool accessible (Ubuntu puts it in version-specific directory)
RUN ln -sf /usr/lib/linux-tools/*/bpftool /usr/local/bin/bpftool || \
		echo "Warning: bpftool symlink failed, will try at runtime"

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

# Install Go 1.23 from official tarball
RUN curl -sL https://go.dev/dl/go1.23.4.linux-amd64.tar.gz | \
		tar -C /usr/local -xz

# Set Go in PATH
ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /workspace
