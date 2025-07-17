// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package ebpf

import (
	"fmt"
	"os"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/go-logr/logr"
)

// CoreManager handles CO-RE program loading and management
type CoreManager struct {
	logger    logr.Logger
	kernelBTF *btf.Spec
	programs  map[string]*ebpf.Program
	links     []link.Link
	mu        sync.RWMutex
}

// CoreCapabilities represents the CO-RE capabilities of the current kernel
type CoreCapabilities struct {
	HasNativeBTF     bool
	HasCORESupport   bool
	HasRingBuf       bool
	HasTracepoints   bool
	HasKprobes       bool
	HasUprobes       bool
	KernelVersion    string
	BTFSource        string
	SupportLevel     string
}

// NewCoreManager creates a new CO-RE manager with kernel BTF discovery
func NewCoreManager(logger logr.Logger) (*CoreManager, error) {
	logger = logger.WithName("core-manager")
	
	// Try to load kernel BTF
	kernelBTF, err := btf.LoadKernelBTF()
	if err != nil {
		logger.Info("Failed to load kernel BTF, trying fallback methods", "error", err)
		
		// Try fallback methods
		kernelBTF, err = loadFallbackBTF(logger)
		if err != nil {
			return nil, fmt.Errorf("failed to load kernel BTF: %w", err)
		}
	}

	logger.Info("Successfully loaded kernel BTF", "types", len(kernelBTF.Types))

	return &CoreManager{
		logger:    logger,
		kernelBTF: kernelBTF,
		programs:  make(map[string]*ebpf.Program),
		links:     make([]link.Link, 0),
	}, nil
}

// loadFallbackBTF attempts to load BTF using fallback methods
func loadFallbackBTF(logger logr.Logger) (*btf.Spec, error) {
	// Try to load from embedded BTF (for older kernels)
	// This would be implemented in the future to include embedded BTF
	// for common kernel versions
	
	// For now, return an error indicating CO-RE is not supported
	return nil, fmt.Errorf("kernel BTF not available and no fallback BTF found")
}

// DetectCapabilities detects the CO-RE capabilities of the current kernel
func (cm *CoreManager) DetectCapabilities() (*CoreCapabilities, error) {
	caps := &CoreCapabilities{
		HasNativeBTF:   false,
		HasCORESupport: false,
		HasRingBuf:     false,
		HasTracepoints: false,
		HasKprobes:     false,
		HasUprobes:     false,
		KernelVersion:  "unknown",
		BTFSource:      "none",
		SupportLevel:   "none",
	}

	// Check for native BTF support
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		caps.HasNativeBTF = true
		caps.BTFSource = "native"
		caps.SupportLevel = "full"
	}

	// Check for CO-RE support (requires BTF)
	if cm.kernelBTF != nil {
		caps.HasCORESupport = true
		if !caps.HasNativeBTF {
			caps.BTFSource = "fallback"
			caps.SupportLevel = "partial"
		}
	}

	// Check for ring buffer support (kernel 5.8+)
	if _, err := os.Stat("/sys/kernel/debug/tracing/events/bpf"); err == nil {
		caps.HasRingBuf = true
	}

	// Check for tracepoint support
	if _, err := os.Stat("/sys/kernel/debug/tracing/events"); err == nil {
		caps.HasTracepoints = true
	}

	// Check for kprobe support
	if _, err := os.Stat("/sys/kernel/debug/tracing/kprobe_events"); err == nil {
		caps.HasKprobes = true
	}

	// Check for uprobe support
	if _, err := os.Stat("/sys/kernel/debug/tracing/uprobe_events"); err == nil {
		caps.HasUprobes = true
	}

	return caps, nil
}

// LoadProgramWithCORE loads an eBPF program with CO-RE relocations
func (cm *CoreManager) LoadProgramWithCORE(spec *ebpf.CollectionSpec) (*ebpf.Collection, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Apply CO-RE relocations
	if err := spec.RewriteConstants(cm.kernelBTF); err != nil {
		return nil, fmt.Errorf("CO-RE relocation failed: %w", err)
	}

	cm.logger.Info("Applied CO-RE relocations", "maps", len(spec.Maps), "programs", len(spec.Programs))

	// Load the collection
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to load eBPF collection: %w", err)
	}

	cm.logger.Info("Successfully loaded eBPF collection with CO-RE support")

	return coll, nil
}

// LoadProgramSpec loads an eBPF program specification from an object file
func (cm *CoreManager) LoadProgramSpec(objectPath string) (*ebpf.CollectionSpec, error) {
	spec, err := ebpf.LoadCollectionSpec(objectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load collection spec from %s: %w", objectPath, err)
	}

	cm.logger.Info("Loaded program specification", "path", objectPath, "maps", len(spec.Maps), "programs", len(spec.Programs))

	return spec, nil
}

// AttachTracepoint attaches a program to a tracepoint
func (cm *CoreManager) AttachTracepoint(prog *ebpf.Program, group, name string) (link.Link, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	l, err := link.Tracepoint(link.TracepointOptions{
		Group:   group,
		Name:    name,
		Program: prog,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach tracepoint %s:%s: %w", group, name, err)
	}

	cm.links = append(cm.links, l)
	cm.logger.Info("Attached tracepoint", "group", group, "name", name)

	return l, nil
}

// AttachKprobe attaches a program to a kprobe
func (cm *CoreManager) AttachKprobe(prog *ebpf.Program, symbol string) (link.Link, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	l, err := link.Kprobe(link.KprobeOptions{
		Symbol:  symbol,
		Program: prog,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach kprobe %s: %w", symbol, err)
	}

	cm.links = append(cm.links, l)
	cm.logger.Info("Attached kprobe", "symbol", symbol)

	return l, nil
}

// AttachKretprobe attaches a program to a kretprobe
func (cm *CoreManager) AttachKretprobe(prog *ebpf.Program, symbol string) (link.Link, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	l, err := link.Kretprobe(link.KretprobeOptions{
		Symbol:  symbol,
		Program: prog,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach kretprobe %s: %w", symbol, err)
	}

	cm.links = append(cm.links, l)
	cm.logger.Info("Attached kretprobe", "symbol", symbol)

	return l, nil
}

// DetachAll detaches all attached programs
func (cm *CoreManager) DetachAll() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var errors []error

	for i, l := range cm.links {
		if err := l.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close link %d: %w", i, err))
		}
	}

	cm.links = cm.links[:0]

	if len(errors) > 0 {
		return fmt.Errorf("failed to detach some programs: %v", errors)
	}

	cm.logger.Info("Detached all programs")
	return nil
}

// GetKernelBTF returns the kernel BTF spec
func (cm *CoreManager) GetKernelBTF() *btf.Spec {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.kernelBTF
}

// Close closes the CoreManager and cleans up resources
func (cm *CoreManager) Close() error {
	return cm.DetachAll()
}

// ValidateCORESupport validates that CO-RE is supported on the current kernel
func ValidateCORESupport(logger logr.Logger) error {
	// Try to load kernel BTF
	_, err := btf.LoadKernelBTF()
	if err != nil {
		return fmt.Errorf("CO-RE not supported: %w", err)
	}

	logger.Info("CO-RE support validated successfully")
	return nil
}

// IsCORESupported returns true if CO-RE is supported on the current kernel
func IsCORESupported() bool {
	_, err := btf.LoadKernelBTF()
	return err == nil
}