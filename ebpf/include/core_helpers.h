// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

#ifndef __CORE_HELPERS_H__
#define __CORE_HELPERS_H__

#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>

// For LINUX_VERSION_CODE
#ifndef LINUX_VERSION_CODE
#define LINUX_VERSION_CODE 0
#endif

// CO-RE field access macros for safer and more portable field access
#define BPF_CORE_READ_VAR(dst, src, field) \
    BPF_CORE_READ_INTO(dst, src, field)

#define BPF_CORE_READ_STR(dst, src, field) \
    BPF_CORE_READ_STR_INTO(dst, src, field)

// Kernel version compatibility checks
// Note: KERNEL_VERSION is already defined in bpf_helpers.h
static __always_inline bool kernel_version_ge(int major, int minor, int patch) {
    return LINUX_VERSION_CODE >= KERNEL_VERSION(major, minor, patch);
}

static __always_inline bool kernel_version_le(int major, int minor, int patch) {
    return LINUX_VERSION_CODE <= KERNEL_VERSION(major, minor, patch);
}

// CO-RE type and field existence checks
#define BPF_CORE_TYPE_EXISTS(type) \
    __builtin_preserve_type_info(*(type*)0, BPF_TYPE_EXISTS)

#define BPF_CORE_FIELD_EXISTS(type, field) \
    __builtin_preserve_field_info(((type*)0)->field, BPF_FIELD_EXISTS)

// CO-RE-safe field size access
#define BPF_CORE_FIELD_SIZE(type, field) \
    __builtin_preserve_field_info(((type*)0)->field, BPF_FIELD_BYTE_SIZE)

// CO-RE-safe field offset access
#define BPF_CORE_FIELD_OFFSET(type, field) \
    __builtin_preserve_field_info(((type*)0)->field, BPF_FIELD_BYTE_OFFSET)

// Helper macros for common kernel structure field access patterns
#define BPF_CORE_READ_TASK_FIELD(dst, task, field) \
    BPF_CORE_READ_INTO(dst, task, field)

#define BPF_CORE_READ_TASK_COMM(dst, task) \
    BPF_CORE_READ_STR_INTO(dst, task, comm)

#define BPF_CORE_READ_TASK_PID(dst, task) \
    BPF_CORE_READ_INTO(dst, task, pid)

#define BPF_CORE_READ_TASK_TGID(dst, task) \
    BPF_CORE_READ_INTO(dst, task, tgid)

// Network-related CO-RE helpers
#define BPF_CORE_READ_SKB_FIELD(dst, skb, field) \
    BPF_CORE_READ_INTO(dst, skb, field)

#define BPF_CORE_READ_NET_DEVICE_NAME(dst, netdev) \
    BPF_CORE_READ_STR_INTO(dst, netdev, name)

// File system CO-RE helpers
#define BPF_CORE_READ_DENTRY_NAME(dst, dentry) \
    BPF_CORE_READ_STR_INTO(dst, dentry, d_name.name)

#define BPF_CORE_READ_INODE_MODE(dst, inode) \
    BPF_CORE_READ_INTO(dst, inode, i_mode)

// CO-RE-safe conditional compilation based on kernel features
#define IF_KERNEL_HAS_FEATURE(feature, code) \
    do { \
        if (BPF_CORE_TYPE_EXISTS(struct feature)) { \
            code \
        } \
    } while (0)

// Common kernel version checks
#define IF_KERNEL_GE(major, minor, patch, code) \
    do { \
        if (kernel_version_ge(major, minor, patch)) { \
            code \
        } \
    } while (0)

#define IF_KERNEL_LE(major, minor, patch, code) \
    do { \
        if (kernel_version_le(major, minor, patch)) { \
            code \
        } \
    } while (0)

// Ring buffer helpers for CO-RE programs
#define BPF_CORE_RINGBUF_RESERVE(map, size) \
    bpf_ringbuf_reserve(map, size, 0)

#define BPF_CORE_RINGBUF_SUBMIT(event, flags) \
    bpf_ringbuf_submit(event, flags)

#define BPF_CORE_RINGBUF_DISCARD(event, flags) \
    bpf_ringbuf_discard(event, flags)

// Map helpers for CO-RE programs
#define BPF_CORE_MAP_LOOKUP_ELEM(map, key) \
    bpf_map_lookup_elem(map, key)

#define BPF_CORE_MAP_UPDATE_ELEM(map, key, value, flags) \
    bpf_map_update_elem(map, key, value, flags)

#define BPF_CORE_MAP_DELETE_ELEM(map, key) \
    bpf_map_delete_elem(map, key)

// Time helpers
#define BPF_CORE_KTIME_GET_NS() \
    bpf_ktime_get_ns()

#define BPF_CORE_KTIME_GET_BOOT_NS() \
    bpf_ktime_get_boot_ns()

// Current task helpers
#define BPF_CORE_GET_CURRENT_TASK() \
    ((struct task_struct *)bpf_get_current_task())

#define BPF_CORE_GET_CURRENT_PID_TGID() \
    bpf_get_current_pid_tgid()

#define BPF_CORE_GET_CURRENT_UID_GID() \
    bpf_get_current_uid_gid()

// Logging helpers for CO-RE programs
#define BPF_CORE_PRINTK(fmt, ...) \
    bpf_printk(fmt, ##__VA_ARGS__)

// CO-RE program section helpers
#define BPF_CORE_SEC(name) SEC(name)

// License declaration helper
#define BPF_CORE_LICENSE(license) \
    char LICENSE[] SEC("license") = license;

// Common CO-RE program patterns
#define BPF_CORE_DEFINE_RINGBUF_MAP(name, size) \
    struct { \
        __uint(type, BPF_MAP_TYPE_RINGBUF); \
        __uint(max_entries, size); \
    } name SEC(".maps")

#define BPF_CORE_DEFINE_HASH_MAP(name, key_type, value_type, max_entries_val) \
    struct { \
        __uint(type, BPF_MAP_TYPE_HASH); \
        __uint(max_entries, max_entries_val); \
        __type(key, key_type); \
        __type(value, value_type); \
    } name SEC(".maps")

#define BPF_CORE_DEFINE_ARRAY_MAP(name, value_type, max_entries) \
    struct { \
        __uint(type, BPF_MAP_TYPE_ARRAY); \
        __uint(max_entries, max_entries); \
        __type(key, __u32); \
        __type(value, value_type); \
    } name SEC(".maps")

#define BPF_CORE_DEFINE_PERCPU_HASH_MAP(name, key_type, value_type, max_entries) \
    struct { \
        __uint(type, BPF_MAP_TYPE_PERCPU_HASH); \
        __uint(max_entries, max_entries); \
        __type(key, key_type); \
        __type(value, value_type); \
    } name SEC(".maps")

// Error handling helpers
#define BPF_CORE_UNLIKELY(x) __builtin_expect(!!(x), 0)
#define BPF_CORE_LIKELY(x) __builtin_expect(!!(x), 1)

// Barrier helpers
#define BPF_CORE_BARRIER() __sync_synchronize()

#endif /* __CORE_HELPERS_H__ */