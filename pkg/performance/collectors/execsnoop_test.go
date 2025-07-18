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

func TestExecSnoopCollector_ParseEvent(t *testing.T) {

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
				event := collectors.ExecsnoopEvent{
					PID:       1234,
					PPID:      1000,
					UID:       1001,
					RetVal:    0,
					ArgsCount: 3,
					ArgsSize:  15, // "arg1\x00arg2\x00arg3\x00" = 15 bytes
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
				event := collectors.ExecsnoopEvent{
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
			name: "event with path in args - command name heuristic",
			buildData: func() []byte {
				var buf bytes.Buffer
				event := collectors.ExecsnoopEvent{
					PID:       9999,
					PPID:      8888,
					UID:       1000,
					RetVal:    0,
					ArgsCount: 2,
					ArgsSize:  41, // "/usr/bin/very-long-command-name\x00--option\x00" = 41 bytes
				}
				copy(event.Comm[:], "very-long-cmd-na") // Truncated at 16 chars

				binary.Write(&buf, binary.LittleEndian, event)
				// Args with full path
				buf.WriteString("/usr/bin/very-long-command-name\x00")
				buf.WriteString("--option\x00")

				return buf.Bytes()
			},
			wantErr: false,
			validate: func(t *testing.T, event *collectors.ExecEvent) {
				assert.Equal(t, int32(9999), event.PID)
				assert.Equal(t, int32(8888), event.PPID)
				assert.Equal(t, uint32(1000), event.UID)
				// Command should be extracted from path, not truncated kernel comm
				assert.Equal(t, "very-long-command-name", event.Command)
				assert.Equal(t, []string{"/usr/bin/very-long-command-name", "--option"}, event.Args)
			},
		},
		{
			name: "event with simple command - keep kernel comm",
			buildData: func() []byte {
				var buf bytes.Buffer
				event := collectors.ExecsnoopEvent{
					PID:       7777,
					PPID:      6666,
					UID:       500,
					RetVal:    0,
					ArgsCount: 1,
					ArgsSize:  8,
				}
				copy(event.Comm[:], "python3")

				binary.Write(&buf, binary.LittleEndian, event)
				// Args without path
				buf.WriteString("python3\x00")

				return buf.Bytes()
			},
			wantErr: false,
			validate: func(t *testing.T, event *collectors.ExecEvent) {
				assert.Equal(t, int32(7777), event.PID)
				assert.Equal(t, int32(6666), event.PPID)
				assert.Equal(t, uint32(500), event.UID)
				// Command should remain as kernel comm since no path in args
				assert.Equal(t, "python3", event.Command)
				assert.Equal(t, []string{"python3"}, event.Args)
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
			logger := logr.Discard()
			config := performance.DefaultCollectionConfig()
			collector, err := collectors.NewExecSnoopCollector(logger, config, "")
			require.NoError(t, err)

			data := tt.buildData()
			event, err := collector.ParseEvent(data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			require.NotNil(t, event)
			if tt.validate != nil {
				tt.validate(t, event)
			}
		})
	}
}

func TestExecSnoopCollector_StartStop(t *testing.T) {
	logger := logr.Discard()
	config := performance.DefaultCollectionConfig()

	// Use a non-existent BPF object path to test error handling
	collector, err := collectors.NewExecSnoopCollector(logger, config, "/nonexistent/execsnoop.bpf.o")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collector.Start(ctx)
	assert.Error(t, err, "Start should fail with missing BPF object or unsupported platform")
	err = collector.Stop()
	assert.NoError(t, err, "Stop should not fail even if Start did")
	err = collector.Stop()
	assert.NoError(t, err, "Multiple Stop calls should not fail")
}

func TestExecSnoopCollector_DoubleStart(t *testing.T) {
	logger := logr.Discard()
	config := performance.DefaultCollectionConfig()

	collector, err := collectors.NewExecSnoopCollector(logger, config, "")
	require.NoError(t, err)

	// Manually set status to active to test double-start protection since we
	// can't actually start without a valid BPF object or root privileges
	collector.SetStatus(performance.CollectorStatusActive)

	ctx := context.Background()
	_, err = collector.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}
