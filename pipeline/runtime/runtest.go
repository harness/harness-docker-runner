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
	"github.com/harness/lite-engine/ti/savings"
	"github.com/harness/ti-client/types"
	"github.com/sirupsen/logrus"
)

const (
	cgDir = "%s/ti/callgraph/" // path where callgraph files will be generated
)

func executeRunTestStep(ctx context.Context, engine *engine.Engine, r *api.StartStepRequest, out io.Writer, tiConfig *tiCfg.Cfg) (
	*runtime.State, map[string]string, []byte, []*api.OutputV2, string, error) {
	start := time.Now()
	log := logrus.New()
	log.Out = out
	optimizationState := types.DISABLED
	cmd, err := instrumentation.GetCmd(ctx, &r.RunTest, r.Name, r.WorkingDir, log, r.Envs, tiConfig)
	if err != nil {
		return nil, nil, nil, nil, string(optimizationState), err
	}

	step := toStep(r)
	step.Command = []string{cmd}
	step.Entrypoint = r.RunTest.Entrypoint

	if (len(r.OutputVars) > 0 || len(r.Outputs) > 0) && len(step.Entrypoint) == 0 || len(step.Command) == 0 {
		return nil, nil, nil, nil, string(optimizationState), fmt.Errorf("output variable should not be set for unset entrypoint or command")
	}

	enablePluginOutputSecrets := IsFeatureFlagEnabled(ciEnablePluginOutputSecrets, engine, step)

	var outputFile string
	if enablePluginOutputSecrets {
		outputFile = fmt.Sprintf("%s/%s-output.env", pipeline.SharedVolPath, step.ID)
	} else {
		outputFile = fmt.Sprintf("%s/%s.out", pipeline.SharedVolPath, step.ID)
	}

	if len(r.Outputs) > 0 {
		step.Command[0] += getOutputsCmd(step.Entrypoint, r.Outputs, outputFile, enablePluginOutputSecrets)
	} else if len(r.OutputVars) > 0 {
		step.Command[0] += getOutputVarCmd(step.Entrypoint, r.OutputVars, outputFile, enablePluginOutputSecrets)
	}

	artifactFile := fmt.Sprintf("%s/%s-artifact", pipeline.SharedVolPath, step.ID)
	step.Envs["PLUGIN_ARTIFACT_FILE"] = artifactFile

	exited, err := engine.Run(ctx, step, out)
	timeTakenMs := time.Since(start).Milliseconds()
	if rerr := report.ParseAndUploadTests(ctx, r.TestReport, r.WorkingDir, step.Name, log, time.Now(), tiConfig, r.Envs); rerr != nil {
		log.WithError(rerr).Errorln("failed to upload report")
	}

	if uerr := callgraph.Upload(ctx, step.Name, time.Since(start).Milliseconds(), log, time.Now(), tiConfig, cgDir); uerr != nil {
		log.WithError(uerr).Errorln("unable to collect callgraph")
	}

	// Parse and upload savings to TI
	if tiConfig.GetParseSavings() {
		optimizationState = savings.ParseAndUploadSavings(ctx, r.WorkingDir, log, step.Name, checkStepSuccess(exited, err), timeTakenMs, tiConfig, r.Envs)
	}

	artifact, _ := fetchArtifactDataFromArtifactFile(artifactFile, out)
	var outputs map[string]string
	var outputErr error
	if len(r.Outputs) > 0 {
		if exited != nil && exited.Exited && exited.ExitCode == 0 {
			if enablePluginOutputSecrets {
				outputs, outputErr = fetchExportedVarsFromEnvFile(outputFile, out) // nolint:govet
			} else {
				outputs, outputErr = fetchOutputVariables(outputFile, out, false) // nolint:govet
			}

			outputsV2 := []*api.OutputV2{}
			for _, output := range r.Outputs {
				if _, ok := outputs[output.Key]; ok {
					outputsV2 = append(outputsV2, &api.OutputV2{
						Key:   output.Key,
						Value: outputs[output.Key],
						Type:  output.Type,
					})
				}
			}
			return exited, outputs, artifact, outputsV2, string(optimizationState), outputErr
		}
	} else if len(r.OutputVars) > 0 {
		if exited != nil && exited.Exited && exited.ExitCode == 0 {
			if enablePluginOutputSecrets {
				outputs, outputErr = fetchExportedVarsFromEnvFile(outputFile, out) // nolint:govet
			} else {
				outputs, outputErr = fetchOutputVariables(outputFile, out, false) // nolint:govet
			}
			return exited, outputs, artifact, nil, string(optimizationState), outputErr
		}
	}
	return exited, nil, artifact, nil, string(optimizationState), err
}
