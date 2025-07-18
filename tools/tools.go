// Copyright Antimetal, Inc. All rights reserved.
//
// Use of this source code is governed by a source available license that can be found in the
// LICENSE file or at:
// https://polyformproject.org/wp-content/uploads/2020/06/PolyForm-Shield-1.0.0.txt

//go:build tools
// +build tools

package tools

import (
	// eBPF code generation
	_ "github.com/cilium/ebpf/cmd/bpf2go"
)