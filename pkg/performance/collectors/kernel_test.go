// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package collectors

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/antimetal/agent/pkg/performance"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKernelCollector_parseKmsgLine(t *testing.T) {
	// Create a test collector
	config := performance.CollectionConfig{
		HostProcPath: t.TempDir(),
		HostDevPath:  "/dev",
	}
	collector := NewKernelCollector(logr.Discard(), config)

	// Create a fake /proc/stat file for boot time calculation
	statContent := "cpu  10 20 30 40 50 60 70 80 90 100\n" +
		"cpu0 1 2 3 4 5 6 7 8 9 10\n" +
		"intr 1234567890\n" +
		"ctxt 9876543210\n" +
		"btime 1640995200\n" + // 2022-01-01 00:00:00 UTC
		"processes 12345\n" +
		"procs_running 1\n" +
		"procs_blocked 0\n"
	err := os.WriteFile(filepath.Join(config.HostProcPath, "stat"), []byte(statContent), 0644)
	require.NoError(t, err)

	bootTime, err := collector.procUtils.GetBootTime()
	require.NoError(t, err)

	tests := []struct {
		name     string
		line     string
		wantErr  bool
		validate func(t *testing.T, msg *performance.KernelMessage)
	}{
		{
			name: "standard kernel message",
			line: "6,1234,5678901234,-;usb 1-1: new high-speed USB device number 2 using xhci_hcd\n",
			validate: func(t *testing.T, msg *performance.KernelMessage) {
				assert.Equal(t, uint8(0), msg.Facility) // 6 >> 3 = 0
				assert.Equal(t, uint8(6), msg.Severity) // 6 & 7 = 6 (INFO)
				assert.Equal(t, uint64(1234), msg.SequenceNum)
				assert.Equal(t, "usb 1-1: new high-speed USB device number 2 using xhci_hcd", msg.Message)
				assert.Equal(t, "usb", msg.Subsystem)
				assert.Equal(t, "1-1", msg.Device)

				// Check timestamp calculation
				expectedTime := bootTime.Add(time.Duration(5678901234) * time.Microsecond)
				assert.Equal(t, expectedTime.Unix(), msg.Timestamp.Unix())
			},
		},
		{
			name: "message with subsystem in brackets",
			line: "4,999,123456789,-;[drm:intel_dp_detect [i915]] DP-1: EDID checksum failed\n",
			validate: func(t *testing.T, msg *performance.KernelMessage) {
				assert.Equal(t, uint8(0), msg.Facility) // 4 >> 3 = 0
				assert.Equal(t, uint8(4), msg.Severity) // 4 & 7 = 4 (WARNING)
				assert.Equal(t, "[drm:intel_dp_detect [i915]] DP-1: EDID checksum failed", msg.Message)
				assert.Equal(t, "drm:intel_dp_detect [i915]", msg.Subsystem)
			},
		},
		{
			name: "emergency message",
			line: "0,100,50000000,-;kernel: Out of memory: Kill process 1234 (chrome) score 999\n",
			validate: func(t *testing.T, msg *performance.KernelMessage) {
				assert.Equal(t, uint8(0), msg.Facility) // 0 >> 3 = 0
				assert.Equal(t, uint8(0), msg.Severity) // 0 & 7 = 0 (EMERGENCY)
				assert.Equal(t, "kernel: Out of memory: Kill process 1234 (chrome) score 999", msg.Message)
				assert.Equal(t, "kernel", msg.Subsystem)
			},
		},
		{
			name:    "invalid format - missing semicolon",
			line:    "6,1234,5678901234,- missing semicolon",
			wantErr: true,
		},
		{
			name:    "invalid format - not enough fields",
			line:    "6,1234;message",
			wantErr: true,
		},
		{
			name:    "invalid priority",
			line:    "abc,1234,5678901234,-;test message",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := collector.parseKmsgLine(tt.line, bootTime)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, msg)

			if tt.validate != nil {
				tt.validate(t, msg)
			}
		})
	}
}

func TestKernelCollector_getBootTime(t *testing.T) {
	tempDir := t.TempDir()

	// Create test /proc/stat file with boot time
	statContent := "cpu  10 20 30 40 50 60 70 80 90 100\n" +
		"cpu0 1 2 3 4 5 6 7 8 9 10\n" +
		"intr 1234567890\n" +
		"ctxt 9876543210\n" +
		"btime 1640995200\n" + // 2022-01-01 00:00:00 UTC
		"processes 12345\n" +
		"procs_running 1\n" +
		"procs_blocked 0\n"
	statPath := filepath.Join(tempDir, "stat")
	err := os.WriteFile(statPath, []byte(statContent), 0644)
	require.NoError(t, err)

	config := performance.CollectionConfig{
		HostProcPath: tempDir,
		HostDevPath:  "/dev",
	}
	collector := NewKernelCollector(logr.Discard(), config)

	// First call should calculate boot time
	bootTime1, err := collector.procUtils.GetBootTime()
	require.NoError(t, err)

	// Boot time should be 2022-01-01 00:00:00 UTC
	expectedBootTime := time.Unix(1640995200, 0)
	assert.Equal(t, expectedBootTime, bootTime1)

	// Second call should return cached value
	bootTime2, err := collector.procUtils.GetBootTime()
	require.NoError(t, err)
	assert.Equal(t, bootTime1, bootTime2)

	// Modify the stat file - cached value should not change
	newStatContent := "cpu  10 20 30 40 50 60 70 80 90 100\n" +
		"cpu0 1 2 3 4 5 6 7 8 9 10\n" +
		"intr 1234567890\n" +
		"ctxt 9876543210\n" +
		"btime 1700000000\n" + // Different time
		"processes 12345\n" +
		"procs_running 1\n" +
		"procs_blocked 0\n"
	err = os.WriteFile(statPath, []byte(newStatContent), 0644)
	require.NoError(t, err)

	bootTime3, err := collector.procUtils.GetBootTime()
	require.NoError(t, err)
	assert.Equal(t, bootTime1, bootTime3) // Still cached
}

func TestKernelCollector_Properties(t *testing.T) {
	config := performance.CollectionConfig{
		HostProcPath: "/proc",
		HostDevPath:  "/dev",
	}
	collector := NewKernelCollector(logr.Discard(), config)

	assert.Equal(t, performance.MetricTypeKernel, collector.Type())
	assert.Equal(t, "Kernel Message Collector", collector.Name())

	caps := collector.Capabilities()
	assert.True(t, caps.SupportsOneShot)
	assert.False(t, caps.SupportsContinuous)
	assert.True(t, caps.RequiresRoot)
	assert.False(t, caps.RequiresEBPF)
	assert.Equal(t, "3.5.0", caps.MinKernelVersion)
}

func TestKernelCollector_MessageLimit(t *testing.T) {
	config := performance.CollectionConfig{
		HostProcPath: "/proc",
		HostDevPath:  "/dev",
	}

	// Test with custom message limit
	collector := NewKernelCollector(logr.Discard(), config, WithMessageLimit(10))
	assert.Equal(t, 10, collector.messageLimit)

	// Test with default
	collector2 := NewKernelCollector(logr.Discard(), config)
	assert.Equal(t, defaultMessageLimit, collector2.messageLimit)

	// Test with invalid limit (should keep default)
	collector3 := NewKernelCollector(logr.Discard(), config, WithMessageLimit(0))
	assert.Equal(t, defaultMessageLimit, collector3.messageLimit)
}
