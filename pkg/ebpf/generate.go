package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-I../../ebpf/include -Wall -Werror" Hello ../../ebpf/src/hello.bpf.c -- -I../../ebpf/include
