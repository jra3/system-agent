// SPDX-License-Identifier: GPL-2.0-only
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include "../include/common.h"
#include "../include/execsnoop_types.h"

char LICENSE[] SEC("license") = "GPL";

#define DEFAULT_MAXARGS 20
#define INVALID_UID ((uid_t)-1)

struct event {
    struct execsnoop_event base;
    char args[FULL_MAX_ARGS_ARR];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24); // 16 MB
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, pid_t);
    __type(value, struct event);
} execs SEC(".maps");

// Use percpu array, struct event is greater than stack limit
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct event);
} heap SEC(".maps");

const volatile int max_args = DEFAULT_MAXARGS;

SEC("tracepoint/syscalls/sys_enter_execve")
int tracepoint__syscalls__sys_enter_execve(struct trace_event_raw_sys_enter *ctx) {
    struct task_struct *task;
    struct event *event;
    const char **args = (const char **)(ctx->args[1]);
    const char *argp;
    pid_t pid;
    uid_t uid;
    int i;
    u32 zero = 0;

    uid = (u32)bpf_get_current_uid_gid();
    pid = bpf_get_current_pid_tgid() >> 32;

    // "Allocate" temporary event
    event = bpf_map_lookup_elem(&heap, &zero);
    if (!event) {
        return 0;
    }
    event->base.pid = 0;
    event->base.ppid = 0;
    event->base.uid = 0;
    event->base.retval = 0;
    event->base.args_count = 0;
    event->base.args_size = 0;
    event->base.pid = pid;
    event->base.uid = uid;

    task = (struct task_struct *)bpf_get_current_task();
    event->base.ppid = BPF_CORE_READ(task, real_parent, tgid);
    
    event->base.args_count = 0;
    event->base.args_size = 0;

    #pragma unroll
    for (i = 0; i < DEFAULT_MAXARGS && i < max_args; i++) {
        argp = NULL;
        bpf_probe_read_user(&argp, sizeof(argp), &args[i]);
        if (!argp) {
            break;
        }

        // Ensure we have enough space for at least one more arg
        if (event->base.args_size >= FULL_MAX_ARGS_ARR - ARGSIZE) {
            break;
        }
        // Calculate remaining space
        int remaining_space = FULL_MAX_ARGS_ARR - event->base.args_size;
        // Truncate if necessary
        int read_size = remaining_space < ARGSIZE ? remaining_space : ARGSIZE;

        int ret = bpf_probe_read_user_str(&event->args[event->base.args_size], 
                                          read_size, argp);
        if (ret > 0) {
            event->base.args_count++;
            event->base.args_size += ret;
        } else {
            break;
        }
    }

    bpf_map_update_elem(&execs, &pid, event, BPF_ANY);

    return 0;
}

SEC("tracepoint/syscalls/sys_exit_execve")
int tracepoint__syscalls__sys_exit_execve(struct trace_event_raw_sys_exit *ctx) {
    pid_t pid;
    struct event *event;
    struct event *e;

    pid = bpf_get_current_pid_tgid() >> 32;
    event = bpf_map_lookup_elem(&execs, &pid);
    if (!event) {
        return 0;
    }
    event->base.retval = ctx->ret;

    // Always allocate the maximum size for the verifier
    e = bpf_ringbuf_reserve(&events, sizeof(struct event), 0);
    if (!e) {
        bpf_map_delete_elem(&execs, &pid);
        return 0;
    }

    // Copy fixed fields
    e->base.pid = event->base.pid;
    e->base.ppid = event->base.ppid;
    e->base.uid = event->base.uid;
    e->base.retval = event->base.retval;
    e->base.args_count = event->base.args_count;
    e->base.args_size = event->base.args_size;

    // Read comm on exit tracepoint since the command name may change during execve
    bpf_get_current_comm(&e->base.comm, sizeof(e->base.comm));

    if (event->base.args_size > 0 && event->base.args_size <= FULL_MAX_ARGS_ARR) {
        bpf_probe_read_kernel(e->args, event->base.args_size, event->args);
    }
    
    bpf_ringbuf_submit(e, 0);
    bpf_map_delete_elem(&execs, &pid);

    return 0;
}