// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

// Package collectors provides performance data collectors for the Antimetal agent.
package collectors

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/antimetal/agent/pkg/ebpf/core"
	"github.com/antimetal/agent/pkg/performance"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/go-logr/logr"
)

// ExecEvent represents a process execution event captured by the BPF program.
// It contains information about newly executed processes including their
// command line arguments, parent process, and execution result.
type ExecEvent struct {
	Timestamp time.Time
	PID       int32
	PPID      int32
	UID       uint32
	RetVal    int32
	Command   string
	Args      []string
}

// ExecSnoopCollector is a continuous collector that monitors process executions
// using eBPF. It attaches to execve syscall tracepoints and captures process
// execution events in real-time through a ring buffer.
type ExecSnoopCollector struct {
	performance.BaseContinuousCollector

	mu            sync.Mutex
	bpfObjectPath string
	coreManager   *core.Manager
	objs          *ebpf.Collection
	enterLink     link.Link
	exitLink      link.Link
	reader        *ringbuf.Reader
	outputChan    chan any
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

func NewExecSnoopCollector(logger logr.Logger, config performance.CollectionConfig, bpfObjectPath string) (*ExecSnoopCollector, error) {
	if bpfObjectPath == "" {
		// Check environment variable first
		bpfObjectPath = os.Getenv("ANTIMETAL_BPF_PATH")
		if bpfObjectPath != "" {
			bpfObjectPath = filepath.Join(bpfObjectPath, "execsnoop.bpf.o")
		} else {
			bpfObjectPath = "/usr/local/lib/antimetal/ebpf/execsnoop.bpf.o"
		}
	}

	collector := &ExecSnoopCollector{
		BaseContinuousCollector: performance.NewBaseContinuousCollector(
			performance.MetricTypeProcess,
			"execsnoop",
			logger,
			config,
			performance.CollectorCapabilities{
				SupportsOneShot:    false,
				SupportsContinuous: true,
				RequiresRoot:       true,
				RequiresEBPF:       true,
				MinKernelVersion:   "5.8", // Ring buffer support
			},
		),
		bpfObjectPath: bpfObjectPath,
		stopChan:      make(chan struct{}),
	}

	return collector, nil
}

func (c *ExecSnoopCollector) Start(ctx context.Context) (<-chan any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Status() == performance.CollectorStatusActive {
		return nil, errors.New("collector already running")
	}

	// Remove memory limit for BPF operations
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("removing memlock: %w", err)
	}

	if c.coreManager == nil {
		manager, err := core.NewManager(c.Logger())
		if err != nil {
			return nil, fmt.Errorf("creating CO-RE manager: %w", err)
		}
		c.coreManager = manager
	}

	coll, err := c.coreManager.LoadCollection(c.bpfObjectPath)
	if err != nil {
		return nil, fmt.Errorf("loading BPF collection with CO-RE: %w", err)
	}
	c.objs = coll

	// Programs loaded?
	enterProg, ok := c.objs.Programs["tracepoint__syscalls__sys_enter_execve"]
	if !ok {
		c.cleanup()
		return nil, errors.New("sys_enter_execve program not found")
	}

	exitProg, ok := c.objs.Programs["tracepoint__syscalls__sys_exit_execve"]
	if !ok {
		c.cleanup()
		return nil, errors.New("sys_exit_execve program not found")
	}

	// Tracepoints attached?
	c.enterLink, err = link.Tracepoint("syscalls", "sys_enter_execve", enterProg, nil)
	if err != nil {
		c.cleanup()
		return nil, fmt.Errorf("attaching enter tracepoint: %w", err)
	}

	c.exitLink, err = link.Tracepoint("syscalls", "sys_exit_execve", exitProg, nil)
	if err != nil {
		c.cleanup()
		return nil, fmt.Errorf("attaching exit tracepoint: %w", err)
	}

	// Maps mapped?
	eventsMap, ok := c.objs.Maps["events"]
	if !ok {
		c.cleanup()
		return nil, errors.New("events map not found")
	}

	c.reader, err = ringbuf.NewReader(eventsMap)
	if err != nil {
		c.cleanup()
		return nil, fmt.Errorf("opening ring buffer: %w", err)
	}

	c.outputChan = make(chan any, 100)

	c.wg.Add(1)
	go c.readEvents(ctx)

	c.SetStatus(performance.CollectorStatusActive)
	return c.outputChan, nil
}

func (c *ExecSnoopCollector) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Status() != performance.CollectorStatusActive {
		return nil
	}

	close(c.stopChan)
	c.wg.Wait()
	c.cleanup()

	if c.outputChan != nil {
		close(c.outputChan)
		c.outputChan = nil
	}

	c.SetStatus(performance.CollectorStatusDisabled)
	return nil
}

func (c *ExecSnoopCollector) cleanup() {
	if c.reader != nil {
		c.reader.Close()
		c.reader = nil
	}

	if c.enterLink != nil {
		c.enterLink.Close()
		c.enterLink = nil
	}

	if c.exitLink != nil {
		c.exitLink.Close()
		c.exitLink = nil
	}

	if c.objs != nil {
		c.objs.Close()
		c.objs = nil
	}
}

func (c *ExecSnoopCollector) readEvents(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		default:
			record, err := c.reader.Read()
			if err != nil {
				if errors.Is(err, ringbuf.ErrClosed) {
					return
				}
				c.SetError(fmt.Errorf("reading from ring buffer: %w", err))
				continue
			}

			event, err := c.parseEvent(record.RawSample)
			if err != nil {
				c.Logger().Error(err, "parsing event")
				continue
			}

			select {
			case c.outputChan <- event:
			case <-ctx.Done():
				return
			case <-c.stopChan:
				return
			default:
				// Channel full, drop event
				c.Logger().V(1).Info("dropping event, channel full")
			}
		}
	}
}

func (c *ExecSnoopCollector) parseEvent(data []byte) (*ExecEvent, error) {
	if len(data) < int(unsafe.Sizeof(ExecsnoopEvent{})) {
		return nil, fmt.Errorf("event too small: %d bytes", len(data))
	}

	// Parse fixed fields
	var raw ExecsnoopEvent
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &raw); err != nil {
		return nil, fmt.Errorf("reading event header: %w", err)
	}

	event := &ExecEvent{
		Timestamp: time.Now(),
		PID:       raw.PID,
		PPID:      raw.PPID,
		UID:       raw.UID,
		RetVal:    raw.RetVal,
		Command:   string(bytes.TrimRight(raw.Comm[:], "\x00")),
		Args:      make([]string, 0, raw.ArgsCount),
	}

	// Parse arguments
	offset := int(unsafe.Sizeof(ExecsnoopEvent{}))
	argsData := data[offset:]

	// Would it be better if this was != instead of >?
	if int(raw.ArgsSize) > len(argsData) {
		return nil, fmt.Errorf("args size mismatch: expected %d, got %d", raw.ArgsSize, len(argsData))
	}

	// Split null-terminated strings
	start := 0
	for i := 0; i < int(raw.ArgsSize) && len(event.Args) < int(raw.ArgsCount); i++ {
		if argsData[i] == 0 {
			if i > start {
				event.Args = append(event.Args, string(argsData[start:i]))
			}
			start = i + 1
		}
	}

	// Command name heuristic: Choose the best representation of the command
	//
	// The kernel provides raw.Comm (truncated to 16 chars) and Args[0] (user input).
	// We use a heuristic to get the most useful command name:
	//
	// Case 1: Args[0] contains '/' (path-like) → extract basename
	//   - "/usr/bin/very-long-command-name" → "very-long-command-name"
	//   - "./script.sh" → "script.sh"
	//   - Avoids kernel's 16-char truncation limit
	//
	// Case 2: Args[0] has no '/' (simple command) → keep raw.Comm
	//   - "python3" → use raw.Comm (which is "python3")
	//   - "ls" → use raw.Comm (which is "ls")
	//   - raw.Comm is authoritative for the actual executable name
	if len(event.Args) > 0 && strings.Contains(event.Args[0], "/") {
		// Extract basename from path
		parts := strings.Split(event.Args[0], "/")
		if len(parts) > 0 {
			event.Command = parts[len(parts)-1]
		}
	}

	return event, nil
}
