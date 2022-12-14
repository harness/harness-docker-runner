// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package report

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/ti/client"
	"github.com/harness/harness-docker-runner/ti/report/parser/junit"
	"github.com/sirupsen/logrus"
)

func ParseAndUploadTests(ctx context.Context, report api.TestReport, workDir, stepID string, log *logrus.Logger, ticlient client.Client) error {
	if report.Kind != api.Junit {
		return fmt.Errorf("unknown report type: %s", report.Kind)
	}

	if len(report.Junit.Paths) == 0 {
		return nil
	}

	// Append working dir to the paths. In k8s, we specify the workDir in the YAML but this is
	// needed in case of VMs.
	for idx, p := range report.Junit.Paths {
		if p[0] != '~' && p[0] != '/' && p[0] != '\\' {
			if !strings.HasPrefix(p, workDir) {
				report.Junit.Paths[idx] = filepath.Join(workDir, p)
			}
		}
	}

	tests := junit.ParseTests(report.Junit.Paths, log)
	if len(tests) == 0 {
		return nil
	}

	return ticlient.Write(ctx, stepID, strings.ToLower(report.Kind.String()), tests)
}
