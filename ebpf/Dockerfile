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
RUN git clone --depth 1 --branch v7.5.0 https://github.com/libbpf/bpftool.git /tmp/bpftool && \
    cd /tmp/bpftool && \
    git submodule update --init && \
    cd src && \
    make -j$(nproc) && \
    make install && \
    cd / && \
    rm -rf /tmp/bpftool

# Download pre-generated vmlinux.h from btfhub
# This provides a vmlinux.h that works for CO-RE across many kernel versions
RUN echo "=== Downloading pre-generated vmlinux.h ===" && \
    curl -sL https://github.com/aquasecurity/btfhub-archive/raw/main/ubuntu/20.04/arm64/5.8.0-63-generic.btf.tar.xz -o /tmp/btf.tar.xz && \
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

# Install Go 1.23 from official tarball
RUN curl -sL https://go.dev/dl/go1.23.4.linux-amd64.tar.gz | \
		tar -C /usr/local -xz

# Set Go in PATH
ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /workspace
