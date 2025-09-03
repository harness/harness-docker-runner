// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/sirupsen/logrus"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/pipeline"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/lite-engine/ti/report"
	"github.com/harness/lite-engine/ti/savings"
	"github.com/harness/ti-client/types"
)

func executeRunStep(ctx context.Context, engine *engine.Engine, r *api.StartStepRequest, out io.Writer, tiConfig *tiCfg.Cfg) (
	*runtime.State, map[string]string, []byte, []*api.OutputV2, string, *types.TelemetryData, error) {
	start := time.Now()
	telemetry := &types.TelemetryData{}
	step := toStep(r)
	step.Command = r.Run.Command
	step.Entrypoint = r.Run.Entrypoint

	optimizationState := types.DISABLED
	if (len(r.OutputVars) > 0 || len(r.Outputs) > 0) && (len(step.Entrypoint) == 0 || len(step.Command) == 0) {
		return nil, nil, nil, nil, string(optimizationState), telemetry, fmt.Errorf("output variable should not be set for unset entrypoint or command")
	}

	enablePluginOutputSecrets := IsFeatureFlagEnabled(ciEnablePluginOutputSecrets, engine, step)

	var outputFile string

	var outputSecretsFile string

	if enablePluginOutputSecrets {
		outputFile = fmt.Sprintf("%s/%s-output.env", pipeline.SharedVolPath, step.ID)
		step.Envs["DRONE_OUTPUT"] = outputFile

		outputSecretsFile = fmt.Sprintf("%s/%s-output-secrets.env", pipeline.SharedVolPath, step.ID)
		step.Envs["HARNESS_OUTPUT_SECRET_FILE"] = outputSecretsFile
	} else {
		outputFile = fmt.Sprintf("%s/%s.out", pipeline.SharedVolPath, step.ID)
		step.Envs["DRONE_OUTPUT"] = outputFile
	}

	if len(r.Outputs) > 0 {
		step.Command[0] += getOutputsCmd(step.Entrypoint, r.Outputs, outputFile, enablePluginOutputSecrets)
	} else if len(r.OutputVars) > 0 {
		step.Command[0] += getOutputVarCmd(step.Entrypoint, r.OutputVars, outputFile, enablePluginOutputSecrets)
	}

	log := logrus.New()
	log.Out = out

	logrus.WithField("step_id", r.ID).WithField("stage_id", r.StageRuntimeID).Infoln("starting step run")

	// Log the command being executed
	printCommand(step, out)

	artifactFile := fmt.Sprintf("%s/%s-artifact", pipeline.SharedVolPath, step.ID)
	step.Envs["PLUGIN_ARTIFACT_FILE"] = artifactFile

	exited, err := engine.Run(ctx, step, out)
	timeTakenMs := time.Since(start).Milliseconds()
	logrus.WithField("step_id", r.ID).WithField("stage_id", r.StageRuntimeID).Traceln("completed step run")
	if _, rerr := report.ParseAndUploadTests(ctx, r.TestReport, r.WorkingDir, step.Name, log, time.Now(), tiConfig, &telemetry.TestIntelligenceMetaData, r.Envs); rerr != nil {
		logrus.WithError(rerr).WithField("step", step.Name).Errorln("failed to upload report")
	}

	// Parse and upload savings to TI
	if tiConfig.GetParseSavings() {
		optimizationState = savings.ParseAndUploadSavings(ctx, r.WorkingDir, log, step.Name, checkStepSuccess(exited, err), timeTakenMs, tiConfig, r.Envs, telemetry)
	}

	//only for git-clone-step
	if buildLangFile, found := r.Envs["PLUGIN_BUILD_TOOL_FILE"]; found {
		err1 := parseBuildInfo(telemetry, r.WorkingDir+"/"+buildLangFile)
		if err1 != nil {
			logrus.WithContext(ctx).WithError(err1).Errorln("failed to parse build info")
		}
	}

	artifact, _ := fetchArtifactDataFromArtifactFile(artifactFile, out)
	summaryOutputs := make(map[string]string)
	if r.TestReport.Junit.Paths != nil && len(r.TestReport.Junit.Paths) > 0 {
		reportSaveErr := report.SaveReportSummaryToOutputs(ctx, tiConfig, step.Name, summaryOutputs, log, r.Envs)
		if reportSaveErr == nil && report.TestSummaryAsOutputEnabled(r.Envs) {
			log.Info("Test summary set as output variables")
		}
	}
	leSummaryOutputsV2 := report.GetSummaryOutputsV2(summaryOutputs, r.Envs)
	summaryOutputsV2 := convertOutputV2(leSummaryOutputsV2)

	if exited != nil && exited.Exited && exited.ExitCode == 0 {
		if enablePluginOutputSecrets {
			outputs, err := fetchExportedVarsFromEnvFile(outputFile, out)
			outputsV2 := []*api.OutputV2{}
			var finalErr error
			if len(r.Outputs) > 0 {
				// only return err when output vars are expected
				finalErr = err
				for _, output := range r.Outputs {
					if _, ok := outputs[output.Key]; ok {
						outputsV2 = append(outputsV2, &api.OutputV2{
							Key:   output.Key,
							Value: outputs[output.Key],
							Type:  output.Type,
						})
					}
				}
			} else {
				if len(r.OutputVars) > 0 {
					// only return err when output vars are expected
					finalErr = err
				}
				for key, value := range outputs {
					output := &api.OutputV2{
						Key:   key,
						Value: value,
						Type:  api.OutputTypeString,
					}
					outputsV2 = append(outputsV2, output)
				}
			}
			// Delete output variable file
			if _, err := os.Stat(outputFile); err == nil {
				if ferr := os.Remove(outputFile); ferr != nil {
					logrus.WithError(ferr).WithField("file", outputFile).Warnln("could not remove output file")
				}
			}

			//checking exported secrets from plugins if any
			if _, err := os.Stat(outputSecretsFile); err == nil {
				secrets, err := fetchExportedVarsFromEnvFile(outputSecretsFile, out)
				if err != nil {
					log.WithError(err).Errorln("error encountered while fetching output secrets from env File")
				}
				for key, value := range secrets {
					output := &api.OutputV2{
						Key:   key,
						Value: value,
						Type:  api.OutputTypeSecret,
					}
					outputsV2 = append(outputsV2, output)
				}
				// Delete output secrets file
				if ferr := os.Remove(outputSecretsFile); ferr != nil {
					logrus.WithError(ferr).WithField("file", outputSecretsFile).Warnln("could not remove output secrets file")
				}
			}

			if report.TestSummaryAsOutputEnabled(r.Envs) {
				outputsV2 = append(outputsV2, summaryOutputsV2...)
			}

			return exited, outputs, artifact, outputsV2, string(optimizationState), telemetry, finalErr

		} else {
			outputs, err := fetchOutputVariables(outputFile, out, false) // nolint:govet
			if err != nil {
				return exited, nil, nil, nil, string(optimizationState), telemetry, err
			}
			// Delete output variable file
			if ferr := os.Remove(outputFile); ferr != nil {
				logrus.WithError(ferr).WithField("file", outputFile).Warnln("could not remove output file")
			}
			if len(r.Outputs) > 0 {
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
				if report.TestSummaryAsOutputEnabled(r.Envs) {
					outputsV2 = append(outputsV2, summaryOutputsV2...)
				}
				return exited, outputs, artifact, outputsV2, string(optimizationState), telemetry, err
			}
			if len(summaryOutputsV2) > 0 && report.TestSummaryAsOutputEnabled(r.Envs) {
				return exited, summaryOutputs, artifact, summaryOutputsV2, string(optimizationState), telemetry, err
			}
			return exited, outputs, artifact, nil, string(optimizationState), telemetry, err
		}
	}
	if len(summaryOutputsV2) == 0 || !report.TestSummaryAsOutputEnabled(r.Envs) {
		return exited, nil, artifact, nil, string(optimizationState), telemetry, err
	}

	return exited, summaryOutputs, artifact, summaryOutputsV2, string(optimizationState), telemetry, err
}

func parseBuildInfo(telemetryData *types.TelemetryData, buildFile string) error {
	if _, err := os.Stat(buildFile); os.IsNotExist(err) {
		return err
	}

	// Read the JSON file containing the cache metrics.
	data, err := os.ReadFile(buildFile)
	if err != nil {
		return err
	}

	// Deserialize the JSON data into the CacheMetrics struct.
	var buildInfo types.BuildInfo
	if err := json.Unmarshal(data, &buildInfo); err != nil {
		return err
	}

	telemetryData.BuildInfo = buildInfo
	return nil
}
