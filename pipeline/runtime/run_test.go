// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"bytes"
	"testing"

	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/stretchr/testify/assert"
)

func boolPtr(b bool) *bool { return &b }

func TestShowScriptInExecutionLogs(t *testing.T) {
	step := &spec.Step{
		Name:    "test-step",
		Command: []string{"echo hello"},
	}

	tests := []struct {
		name           string
		flag           *bool
		expectPrinted  bool
	}{
		{
			name:          "nil (field absent / older manager) → show script",
			flag:          nil,
			expectPrinted: true,
		},
		{
			name:          "explicit true → show script",
			flag:          boolPtr(true),
			expectPrinted: true,
		},
		{
			name:          "explicit false → hide script",
			flag:          boolPtr(false),
			expectPrinted: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := new(bytes.Buffer)

			// Mirror the exact condition used in executeRunStep.
			if tc.flag == nil || *tc.flag {
				printCommand(step, buf)
			}

			output := stripAnsiCodes(buf.String())
			if tc.expectPrinted {
				assert.Contains(t, output, "echo hello", "expected script in logs")
			} else {
				assert.NotContains(t, output, "echo hello", "expected script to be hidden")
			}
		})
	}
}
