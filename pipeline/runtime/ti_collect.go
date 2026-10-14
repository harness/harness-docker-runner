// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/lite-engine/ti/callgraph"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/lite-engine/ti/report"
	"github.com/harness/ti-client/types"
)

// Indirections so the report/callgraph collection can be unit tested.
var (
	parseAndUploadTestsFn = report.ParseAndUploadTests
	uploadCallgraphFn     = callgraph.Upload
)

// collectTestReportsAndCallgraph parses and uploads the test reports and then uploads the
// callgraph. The parsed tests MUST be passed to the callgraph upload: it uses them to compute
// totalTests, pathToTestNumMap and the per-chain test state. Passing nil makes every chain
// look successful and every test file look like it has exactly one test.
func collectTestReportsAndCallgraph(
	ctx context.Context,
	log *logrus.Logger,
	r *api.StartStepRequest,
	stepName string,
	start time.Time,
	tiConfig *tiCfg.Cfg,
	telemetry *types.TelemetryData,
	cgDirFormat string,
) {
	tests, rerr := parseAndUploadTestsFn(ctx, r.TestReport, r.WorkingDir, stepName, log, time.Now(), tiConfig, &telemetry.TestIntelligenceMetaData, r.Envs)
	if rerr != nil {
		log.WithError(rerr).Errorln("failed to upload report")
	}

	// Passing default false for failed test rerun for now.
	leStepRequest := toLiteEngineStartStepRequest(r)
	if _, uerr := uploadCallgraphFn(ctx, stepName, time.Since(start).Milliseconds(), log, time.Now(), tiConfig, cgDirFormat, r.ID, tests, false, leStepRequest); uerr != nil {
		log.WithError(uerr).Errorln("unable to collect callgraph")
	}
}
