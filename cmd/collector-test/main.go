// Copyright 2025 Antimetal Inc.
//
// Licensed under the PolyForm Shield License 1.0.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     https://polyformproject.org/licenses/shield/1.0.0/
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/antimetal/agent/pkg/performance"
)

var (
	interval    = flag.Duration("interval", 5*time.Second, "Collection interval")
	procPath    = flag.String("proc-path", "/proc", "Path to proc filesystem")
	sysPath     = flag.String("sys-path", "/sys", "Path to sys filesystem")
	devPath     = flag.String("dev-path", "/dev", "Path to dev filesystem")
	verbose     = flag.Bool("verbose", false, "Enable verbose logging")
	metricTypes = flag.String("metrics", "", "Comma-separated list of metric types to collect (empty for all)")
	prettyPrint = flag.Bool("pretty", true, "Pretty print JSON output")
)

func main() {
	flag.Parse()

	// Setup logger
	var logger logr.Logger
	if *verbose {
		zapLog, _ := zap.NewDevelopment()
		logger = zapr.NewLogger(zapLog)
	} else {
		logger = logr.Discard()
	}

	// Create collection config
	config := performance.CollectionConfig{
		HostProcPath: *procPath,
		HostSysPath:  *sysPath,
		HostDevPath:  *devPath,
	}

	// Get available metric types from registry
	availableTypes := []performance.MetricType{
		performance.MetricTypeLoad,
		performance.MetricTypeMemory,
		performance.MetricTypeCPU,
		performance.MetricTypeProcess,
		performance.MetricTypeDisk,
		performance.MetricTypeNetwork,
		performance.MetricTypeTCP,
		performance.MetricTypeKernel,
		performance.MetricTypeCPUInfo,
		performance.MetricTypeMemoryInfo,
		performance.MetricTypeDiskInfo,
		performance.MetricTypeNetworkInfo,
	}

	// Filter metric types if specified
	if *metricTypes != "" {
		requestedTypes := strings.Split(*metricTypes, ",")
		var filteredTypes []performance.MetricType
		for _, requested := range requestedTypes {
			requested = strings.TrimSpace(requested)
			for _, available := range availableTypes {
				if string(available) == requested {
					filteredTypes = append(filteredTypes, available)
					break
				}
			}
		}
		availableTypes = filteredTypes
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Collection ticker
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	fmt.Printf("Starting collector test (interval: %v)\n", *interval)
	fmt.Printf("Testing metric types: %v\n", availableTypes)
	fmt.Printf("Press Ctrl+C to stop\n\n")

	for {
		select {
		case <-ticker.C:
			collectAndPrint(availableTypes, config, logger)
		case <-sigChan:
			fmt.Println("\nStopping collector test...")
			return
		case <-ctx.Done():
			return
		}
	}
}

func collectAndPrint(metricTypes []performance.MetricType, config performance.CollectionConfig, logger logr.Logger) {
	fmt.Printf("=== Collection at %s ===\n", time.Now().Format(time.RFC3339))

	for _, metricType := range metricTypes {
		fmt.Printf("\n--- %s ---\n", metricType)

		// Get collector factory from registry
		factory, err := performance.GetCollector(metricType)
		if err != nil {
			fmt.Printf("Error getting collector: %v\n", err)
			continue
		}

		// Create collector instance
		collector, err := factory(logger, config)
		if err != nil {
			fmt.Printf("Error creating collector: %v\n", err)
			continue
		}

		// Start the collector and get one data point
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		dataChan, err := collector.Start(ctx)
		if err != nil {
			fmt.Printf("Error starting collector: %v\n", err)
			cancel()
			continue
		}

		// Get first data point with timeout
		select {
		case data := <-dataChan:
			if data != nil {
				var output []byte
				var marshalErr error

				if *prettyPrint {
					output, marshalErr = json.MarshalIndent(data, "", "  ")
				} else {
					output, marshalErr = json.Marshal(data)
				}

				if marshalErr != nil {
					fmt.Printf("Error marshaling data: %v\n", marshalErr)
				} else {
					fmt.Printf("%s\n", output)
				}
			} else {
				fmt.Printf("No data received\n")
			}
		case <-time.After(5 * time.Second):
			fmt.Printf("Timeout waiting for data\n")
		}

		// Stop the collector
		collector.Stop()
		cancel()
	}

	fmt.Printf("\n%s\n\n", strings.Repeat("=", 50))
}
