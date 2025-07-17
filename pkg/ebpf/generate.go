package ebpf

// eBPF bindings generation
// Add go:generate directives here when eBPF programs are added to ebpf/src/
// Example:
// //go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-I../../ebpf/include -Wall -Werror" Hello ../../ebpf/src/hello.bpf.c -- -I../../ebpf/include
