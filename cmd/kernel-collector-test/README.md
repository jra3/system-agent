# Kernel Collector Test Harness

A standalone test harness for testing the kernel message collector on Linux systems.

## Building

```bash
go build -o kernel-collector-test cmd/kernel-collector-test/main.go
```

## Usage

### Point-in-time Collection (default)

Collect the last N kernel messages:

```bash
# Collect last 50 messages (default)
sudo ./kernel-collector-test

# Collect last 100 messages
sudo ./kernel-collector-test -limit 100

# Verbose output
sudo ./kernel-collector-test -v
```

### Continuous Collection

Stream kernel messages in real-time:

```bash
# Start continuous collection
sudo ./kernel-collector-test -mode continuous

# Or use the shorthand
sudo ./kernel-collector-test -continuous
```

### Options

- `-mode`: Collection mode (`point` or `continuous`). Default: `point`
- `-limit`: Number of messages to collect in point mode. Default: `50`
- `-proc`: Path to proc filesystem. Default: `/proc`
- `-dev`: Path to dev filesystem. Default: `/dev`
- `-v`: Enable verbose logging
- `-continuous`: Shorthand for `-mode continuous`

## Examples

```bash
# Collect last 20 kernel messages with verbose logging
sudo ./kernel-collector-test -limit 20 -v

# Run continuous collection on a custom dev path
sudo ./kernel-collector-test -continuous -dev /host/dev

# Test with custom proc/dev paths (useful in containers)
sudo ./kernel-collector-test -proc /host/proc -dev /host/dev
```

## Requirements

- Linux kernel 3.5+ (when `/dev/kmsg` was introduced)
- Root privileges or `CAP_SYSLOG` capability
- Go 1.18+ (for generics support)

## Message Format

Messages are displayed as:
```
[NUM] TIMESTAMP SEVERITY [SUBSYSTEM DEVICE] MESSAGE
```

Example:
```
[1] 2024-01-15 10:23:45.123 INFO  [usb 1-1] new high-speed USB device number 2 using xhci_hcd
[2] 2024-01-15 10:23:46.456 WARN  [ext4] EXT4-fs (sda1): warning: mounting unchecked fs
```

## Troubleshooting

### Permission Denied

If you get permission errors, make sure to:
1. Run with `sudo` or as root
2. Or add `CAP_SYSLOG` capability: `sudo setcap cap_syslog+ep ./kernel-collector-test`

### No Messages

If no messages appear:
- Check if `/dev/kmsg` exists
- Verify kernel version is 3.5+
- Try generating some kernel messages: `sudo modprobe -r usbhid && sudo modprobe usbhid`