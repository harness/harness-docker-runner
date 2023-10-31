// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/harness/lite-engine/ti/callgraph"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/lite-engine/ti/instrumentation"
	"github.com/harness/lite-engine/ti/report"
	"github.com/sirupsen/logrus"
)

func executeRunTestStep(ctx context.Context, engine *engine.Engine, r *api.StartStepRequest, out io.Writer, tiConfig *tiCfg.Cfg) (
	*runtime.State, map[string]string, []byte, error) {
	log := logrus.New()
	log.Out = out

	start := time.Now()
	cmd, err := instrumentation.GetCmd(ctx, &r.RunTest, r.Name, r.WorkingDir, log, r.Envs, tiConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	step := toStep(r)
	step.Command = []string{cmd}
	step.Entrypoint = r.RunTest.Entrypoint

	if len(r.OutputVars) > 0 && len(step.Entrypoint) == 0 || len(step.Command) == 0 {
		return nil, nil, nil, fmt.Errorf("output variable should not be set for unset entrypoint or command")
	}

	outputFile := fmt.Sprintf("%s/%s.out", pipeline.SharedVolPath, step.ID)
	if len(r.OutputVars) > 0 {
		step.Command[0] += getOutputVarCmd(step.Entrypoint, r.OutputVars, outputFile)
	}

	artifactFile := fmt.Sprintf("%s/%s-artifact", pipeline.SharedVolPath, step.ID)
	step.Envs["PLUGIN_ARTIFACT_FILE"] = artifactFile

	exited, err := engine.Run(ctx, step, out)
	if rerr := report.ParseAndUploadTests(ctx, r.TestReport, r.WorkingDir, step.Name, log, time.Now(), tiConfig); rerr != nil {
		log.WithError(rerr).Errorln("failed to upload report")
	}

	if uerr := callgraph.Upload(ctx, step.Name, time.Since(start).Milliseconds(), log, time.Now(), tiConfig); uerr != nil {
		log.WithError(uerr).Errorln("unable to collect callgraph")
	}

	artifact, _ := fetchArtifactDataFromArtifactFile(artifactFile, out)
	if len(r.OutputVars) > 0 {
		if exited != nil && exited.Exited && exited.ExitCode == 0 {
			outputs, err := fetchOutputVariables(outputFile, out) // nolint:govet
			if err != nil {
				return exited, nil, nil, err
			}
			return exited, outputs, artifact, err
		}
	}

	return exited, nil, artifact, err
}
