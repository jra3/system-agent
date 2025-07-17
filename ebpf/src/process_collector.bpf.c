// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

#include <vmlinux.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include "core_helpers.h"

// CO-RE-compatible event structure for process monitoring
struct process_event {
    __u32 pid;
    __u32 ppid;
    __u32 uid;
    __u32 gid;
    char comm[16];
    __u64 timestamp;
    __u32 exit_code;
    __u8 event_type; // 0=fork, 1=exec, 2=exit
};

// Ring buffer for process events
BPF_CORE_DEFINE_RINGBUF_MAP(process_events, 256 * 1024);

// Statistics map for tracking process counts
BPF_CORE_DEFINE_HASH_MAP(process_stats, __u32, __u64, 1024);

// Helper function to get process statistics
static __always_inline void update_process_stats(__u32 pid) {
    __u64 *count = BPF_CORE_MAP_LOOKUP_ELEM(&process_stats, &pid);
    if (count) {
        __sync_fetch_and_add(count, 1);
    } else {
        __u64 initial_count = 1;
        BPF_CORE_MAP_UPDATE_ELEM(&process_stats, &pid, &initial_count, BPF_ANY);
    }
}

// Helper function to populate common event fields using CO-RE
static __always_inline int populate_event_common(struct process_event *event, struct task_struct *task) {
    // Use CO-RE helpers to safely access task_struct fields
    BPF_CORE_READ_TASK_PID(&event->pid, task);
    BPF_CORE_READ_TASK_COMM(event->comm, task);
    
    // Read parent PID using CO-RE
    struct task_struct *parent = BPF_CORE_READ(task, real_parent);
    if (parent) {
        BPF_CORE_READ_TASK_PID(&event->ppid, parent);
    }
    
    // Read credentials using CO-RE
    const struct cred *cred = BPF_CORE_READ(task, real_cred);
    if (cred) {
        BPF_CORE_READ_INTO(&event->uid, cred, uid.val);
        BPF_CORE_READ_INTO(&event->gid, cred, gid.val);
    }
    
    event->timestamp = BPF_CORE_KTIME_GET_NS();
    return 0;
}

// Process fork tracepoint
BPF_CORE_SEC("tracepoint/sched/sched_process_fork")
int trace_process_fork(void *ctx) {
    struct process_event *event;
    struct task_struct *parent;
    
    // Reserve space in ring buffer
    event = BPF_CORE_RINGBUF_RESERVE(&process_events, sizeof(*event));
    if (!event) {
        return 0;
    }
    
    // Get current task (parent)
    parent = BPF_CORE_GET_CURRENT_TASK();
    
    // Populate common event fields
    if (populate_event_common(event, parent) < 0) {
        BPF_CORE_RINGBUF_DISCARD(event, 0);
        return 0;
    }
    
    event->event_type = 0; // fork
    event->exit_code = 0;
    
    // Update statistics
    update_process_stats(event->pid);
    
    // Submit event
    BPF_CORE_RINGBUF_SUBMIT(event, 0);
    return 0;
}

// Process exec tracepoint
BPF_CORE_SEC("tracepoint/sched/sched_process_exec")
int trace_process_exec(void *ctx) {
    struct process_event *event;
    struct task_struct *task;
    
    // Reserve space in ring buffer
    event = BPF_CORE_RINGBUF_RESERVE(&process_events, sizeof(*event));
    if (!event) {
        return 0;
    }
    
    // Get current task
    task = BPF_CORE_GET_CURRENT_TASK();
    
    // Populate common event fields
    if (populate_event_common(event, task) < 0) {
        BPF_CORE_RINGBUF_DISCARD(event, 0);
        return 0;
    }
    
    event->event_type = 1; // exec
    event->exit_code = 0;
    
    // Update statistics
    update_process_stats(event->pid);
    
    // Submit event
    BPF_CORE_RINGBUF_SUBMIT(event, 0);
    return 0;
}

// Process exit tracepoint
BPF_CORE_SEC("tracepoint/sched/sched_process_exit")
int trace_process_exit(void *ctx) {
    struct process_event *event;
    struct task_struct *task;
    
    // Reserve space in ring buffer
    event = BPF_CORE_RINGBUF_RESERVE(&process_events, sizeof(*event));
    if (!event) {
        return 0;
    }
    
    // Get current task
    task = BPF_CORE_GET_CURRENT_TASK();
    
    // Populate common event fields
    if (populate_event_common(event, task) < 0) {
        BPF_CORE_RINGBUF_DISCARD(event, 0);
        return 0;
    }
    
    event->event_type = 2; // exit
    
    // Read exit code using CO-RE
    BPF_CORE_READ_INTO(&event->exit_code, task, exit_code);
    
    // Update statistics
    update_process_stats(event->pid);
    
    // Submit event
    BPF_CORE_RINGBUF_SUBMIT(event, 0);
    return 0;
}

// Kernel version-specific functionality
#ifdef CORE_SUPPORT_FULL
// Advanced CO-RE features for newer kernels
BPF_CORE_SEC("kprobe/do_fork")
int kprobe_do_fork(struct pt_regs *ctx) {
    // Only compile this for kernels that support it
    IF_KERNEL_GE(4, 18, 0, {
        struct task_struct *task = BPF_CORE_GET_CURRENT_TASK();
        __u32 pid;
        BPF_CORE_READ_TASK_PID(&pid, task);
        
        // Update statistics for kprobe-based tracking
        update_process_stats(pid);
    });
    
    return 0;
}
#endif

BPF_CORE_LICENSE("GPL");