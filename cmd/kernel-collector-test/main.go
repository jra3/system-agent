// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/antimetal/agent/pkg/performance"
	"github.com/antimetal/agent/pkg/performance/collectors"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

var (
	mode         = flag.String("mode", "point", "Collection mode: 'point' or 'continuous'")
	messageLimit = flag.Int("limit", 50, "Number of messages to collect (point mode only)")
	procPath     = flag.String("proc", "/proc", "Path to proc filesystem")
	devPath      = flag.String("dev", "/dev", "Path to dev filesystem")
	verbose      = flag.Bool("v", false, "Enable verbose logging")
	continuous   = flag.Bool("continuous", false, "Run in continuous mode (alias for -mode=continuous)")
)

func main() {
	flag.Parse()

	// Setup logger
	var logger logr.Logger
	if *verbose {
		zapLogger, _ := zap.NewDevelopment()
		logger = zapr.NewLogger(zapLogger)
	} else {
		zapLogger, _ := zap.NewProduction()
		logger = zapr.NewLogger(zapLogger)
	}

	// Override mode if continuous flag is set
	if *continuous {
		*mode = "continuous"
	}

	// Check if running as root or with CAP_SYSLOG
	if os.Geteuid() != 0 {
		log.Println("Warning: Not running as root. You may need root privileges or CAP_SYSLOG to read /dev/kmsg")
	}

	// Create configuration
	config := performance.CollectionConfig{
		HostProcPath: *procPath,
		HostDevPath:  *devPath,
	}

	// Create collector with options
	collector := collectors.NewKernelCollector(
		logger,
		config,
		collectors.WithMessageLimit(*messageLimit),
	)

	ctx := context.Background()

	switch *mode {
	case "point":
		runPointCollection(ctx, collector)
	case "continuous":
		runContinuousCollection(ctx, collector)
	default:
		log.Fatalf("Invalid mode: %s. Use 'point' or 'continuous'", *mode)
	}
}

func runPointCollection(ctx context.Context, collector *collectors.KernelCollector) {
	fmt.Println("Running point-in-time collection...")
	fmt.Printf("Collecting last %d kernel messages...\n", *messageLimit)
	fmt.Println("---")

	// Perform collection
	result, err := collector.Collect(ctx)
	if err != nil {
		log.Fatalf("Collection failed: %v", err)
	}

	// Type assert to get messages
	messages, ok := result.([]*performance.KernelMessage)
	if !ok {
		log.Fatalf("Unexpected result type: %T", result)
	}

	fmt.Printf("Collected %d messages:\n\n", len(messages))

	// Display messages
	for i, msg := range messages {
		printMessage(i+1, msg)
	}

	if len(messages) == 0 {
		fmt.Println("No kernel messages found. This could mean:")
		fmt.Println("- You don't have permission to read /dev/kmsg")
		fmt.Println("- The kernel ring buffer is empty")
		fmt.Println("- Try running with sudo or as root")
	}
}

func runContinuousCollection(ctx context.Context, collector *collectors.KernelCollector) {
	fmt.Println("Starting continuous collection...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("---")

	// Setup signal handling
	ctx, cancel := context.WithCancel(ctx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start continuous collection
	msgChan, err := collector.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start continuous collection: %v", err)
	}

	// Message counter
	count := 0

	// Run until interrupted
	for {
		select {
		case <-sigChan:
			fmt.Println("\nStopping collection...")
			cancel()
			if err := collector.Stop(); err != nil {
				log.Printf("Error stopping collector: %v", err)
			}
			fmt.Printf("\nTotal messages collected: %d\n", count)
			return

		case msg := <-msgChan:
			if msg == nil {
				fmt.Println("Channel closed, exiting...")
				return
			}

			// Type assert to kernel message
			kmsg, ok := msg.(*performance.KernelMessage)
			if !ok {
				log.Printf("Unexpected message type: %T", msg)
				continue
			}

			count++
			printMessage(count, kmsg)
		}
	}
}

func printMessage(num int, msg *performance.KernelMessage) {
	// Format severity as string
	severity := getSeverityString(msg.Severity)
	
	// Format timestamp
	timestamp := msg.Timestamp.Format("2006-01-02 15:04:05.000")
	
	// Build output
	fmt.Printf("[%d] %s %s", num, timestamp, severity)
	
	// Add subsystem/device if available
	if msg.Subsystem != "" {
		fmt.Printf(" [%s", msg.Subsystem)
		if msg.Device != "" {
			fmt.Printf(" %s", msg.Device)
		}
		fmt.Printf("]")
	}
	
	// Add message
	fmt.Printf(" %s\n", msg.Message)
}

func getSeverityString(severity uint8) string {
	switch severity {
	case 0:
		return "EMERG"
	case 1:
		return "ALERT"
	case 2:
		return "CRIT "
	case 3:
		return "ERROR"
	case 4:
		return "WARN "
	case 5:
		return "NOTICE"
	case 6:
		return "INFO "
	case 7:
		return "DEBUG"
	default:
		return "UNKNOWN"
	}
}