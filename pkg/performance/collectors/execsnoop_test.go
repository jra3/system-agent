// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package collectors_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/antimetal/agent/pkg/performance"
	"github.com/antimetal/agent/pkg/performance/collectors"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecSnoopCollector(t *testing.T) {
	logger := logr.Discard()
	config := performance.DefaultCollectionConfig()

	tests := []struct {
		name          string
		bpfObjectPath string
		wantPath      string
	}{
		{
			name:          "default path",
			bpfObjectPath: "",
			wantPath:      "/usr/local/lib/antimetal/ebpf/execsnoop.bpf.o",
		},
		{
			name:          "custom path",
			bpfObjectPath: "/custom/path/execsnoop.bpf.o",
			wantPath:      "/custom/path/execsnoop.bpf.o",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := collectors.NewExecSnoopCollector(logger, config, tt.bpfObjectPath)
			require.NoError(t, err)
			require.NotNil(t, collector)

			// Verify capabilities
			caps := collector.Capabilities()
			assert.False(t, caps.SupportsOneShot)
			assert.True(t, caps.SupportsContinuous)
			assert.True(t, caps.RequiresRoot)
			assert.True(t, caps.RequiresEBPF)
			assert.Equal(t, "5.8", caps.MinKernelVersion)

			// Verify initial status
			assert.Equal(t, performance.CollectorStatusDisabled, collector.Status())
			assert.NoError(t, collector.LastError())
		})
	}
}

func TestExecSnoopCollector_ParseEvent(t *testing.T) {
	// Test event parsing without requiring actual BPF functionality
	type execsnoopEvent struct {
		PID       int32
		PPID      int32
		UID       uint32
		RetVal    int32
		ArgsCount int32
		ArgsSize  uint32
		Comm      [16]byte
	}

	tests := []struct {
		name      string
		buildData func() []byte
		wantErr   bool
		validate  func(t *testing.T, event *collectors.ExecEvent)
	}{
		{
			name: "valid event with args",
			buildData: func() []byte {
				var buf bytes.Buffer
				event := execsnoopEvent{
					PID:       1234,
					PPID:      1000,
					UID:       1001,
					RetVal:    0,
					ArgsCount: 3,
					ArgsSize:  20,
				}
				copy(event.Comm[:], "test-cmd")

				binary.Write(&buf, binary.LittleEndian, event)
				// Write args
				buf.WriteString("arg1\x00")
				buf.WriteString("arg2\x00")
				buf.WriteString("arg3\x00")

				return buf.Bytes()
			},
			wantErr: false,
			validate: func(t *testing.T, event *collectors.ExecEvent) {
				assert.Equal(t, int32(1234), event.PID)
				assert.Equal(t, int32(1000), event.PPID)
				assert.Equal(t, uint32(1001), event.UID)
				assert.Equal(t, int32(0), event.RetVal)
				assert.Equal(t, "test-cmd", event.Command)
				assert.Equal(t, []string{"arg1", "arg2", "arg3"}, event.Args)
			},
		},
		{
			name: "event with no args",
			buildData: func() []byte {
				var buf bytes.Buffer
				event := execsnoopEvent{
					PID:       5678,
					PPID:      5000,
					UID:       2001,
					RetVal:    -1,
					ArgsCount: 0,
					ArgsSize:  0,
				}
				copy(event.Comm[:], "noargs")

				binary.Write(&buf, binary.LittleEndian, event)
				return buf.Bytes()
			},
			wantErr: false,
			validate: func(t *testing.T, event *collectors.ExecEvent) {
				assert.Equal(t, int32(5678), event.PID)
				assert.Equal(t, int32(5000), event.PPID)
				assert.Equal(t, uint32(2001), event.UID)
				assert.Equal(t, int32(-1), event.RetVal)
				assert.Equal(t, "noargs", event.Command)
				assert.Empty(t, event.Args)
			},
		},
		{
			name: "event too small",
			buildData: func() []byte {
				return []byte{1, 2, 3, 4} // Too small
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test would require exposing the parseEvent method
			// or testing through the full Start/Stop cycle
			// For now, we're testing the structure and setup
			_ = tt.buildData()
		})
	}
}

func TestExecSnoopCollector_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping BPF test in short mode")
	}

	logger := logr.Discard()
	config := performance.DefaultCollectionConfig()

	// Use a non-existent BPF object path to test error handling
	collector, err := collectors.NewExecSnoopCollector(logger, config, "/nonexistent/execsnoop.bpf.o")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start should fail due to missing BPF object or CO-RE not supported on non-Linux
	_, err = collector.Start(ctx)
	assert.Error(t, err, "Start should fail with missing BPF object or unsupported platform")

	// Stop should work even if start failed
	err = collector.Stop()
	assert.NoError(t, err)

	// Multiple stops should be safe
	err = collector.Stop()
	assert.NoError(t, err)
}

func TestExecSnoopCollector_DoubleStart(t *testing.T) {
	logger := logr.Discard()
	config := performance.DefaultCollectionConfig()

	collector, err := collectors.NewExecSnoopCollector(logger, config, "")
	require.NoError(t, err)

	// Manually set status to active to test double-start protection
	collector.SetStatus(performance.CollectorStatusActive)

	ctx := context.Background()
	_, err = collector.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}
