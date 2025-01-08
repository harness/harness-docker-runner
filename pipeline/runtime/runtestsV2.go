// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/sirupsen/logrus"

	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/pipeline"
	leRuntime "github.com/harness/lite-engine/pipeline/runtime"
	"github.com/harness/lite-engine/ti/callgraph"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/lite-engine/ti/report"
	"github.com/harness/lite-engine/ti/savings"
	"github.com/harness/ti-client/types"
)

const (
	outDir = "%s/ti/v2/callgraph/cg/"
)

func executeRunTestsV2Step(ctx context.Context, engine *engine.Engine, r *api.StartStepRequest, out io.Writer, tiConfig *tiCfg.Cfg) (
	*runtime.State, map[string]string, []byte, []*api.OutputV2, string, error) {
	start := time.Now()
	log := logrus.New()
	log.Out = out
	step := toStep(r)
	optimizationState := types.DISABLED
	step.Entrypoint = r.RunTestsV2.Entrypoint
	preCmd, err := leRuntime.SetupRunTestV2(ctx, &r.RunTestsV2, step.Name, r.WorkingDir, log, r.Envs, tiConfig)
	if err != nil {
		return nil, nil, nil, nil, string(optimizationState), err
	}
	command := r.RunTestsV2.Command[0]
	if preCmd != "" {
		command = fmt.Sprintf("%s\n%s", preCmd, command)
	}
	step.Command = []string{command}

	if (len(r.OutputVars) > 0 || len(r.Outputs) > 0) && (len(step.Entrypoint) == 0 || len(step.Command) == 0) {
		return nil, nil, nil, nil, string(optimizationState), fmt.Errorf("output variable should not be set for unset entrypoint or command")
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

	logrus.WithField("step_id", r.ID).WithField("stage_id", r.StageRuntimeID).Traceln("starting step run")

	artifactFile := fmt.Sprintf("%s/%s-artifact", pipeline.SharedVolPath, step.ID)
	step.Envs["PLUGIN_ARTIFACT_FILE"] = artifactFile

	exited, err := engine.Run(ctx, step, out)
	timeTakenMs := time.Since(start).Milliseconds()
	logrus.WithField("step_id", r.ID).WithField("stage_id", r.StageRuntimeID).Traceln("completed step runtestv2")

	if len(r.TestReport.Junit.Paths) == 0 {
		// If there are no paths specified, set Paths[0] to include all XML files and all TRX files
		r.TestReport.Junit.Paths = []string{"**/*.xml", "**/*.trx"}
	}
	if rerr := report.ParseAndUploadTests(ctx, r.TestReport, r.WorkingDir, step.Name, log, time.Now(), tiConfig, r.Envs); rerr != nil {
		log.WithError(rerr).Errorln("failed to upload report")
	}

	if uerr := callgraph.Upload(ctx, step.Name, time.Since(start).Milliseconds(), log, time.Now(), tiConfig, outDir); uerr != nil {
		log.WithError(uerr).Errorln("unable to collect callgraph")
	}

	// Parse and upload savings to TI
	if tiConfig.GetParseSavings() {
		optimizationState = savings.ParseAndUploadSavings(ctx, r.WorkingDir, log, step.Name, checkStepSuccess(exited, err), timeTakenMs, tiConfig, r.Envs)
	}

	artifact, _ := fetchArtifactDataFromArtifactFile(artifactFile, out)
	summaryOutputs := make(map[string]string)
	reportSaveErr := report.SaveReportSummaryToOutputs(ctx, tiConfig, step.Name, summaryOutputs, log, r.Envs)
	if reportSaveErr != nil {
		log.Errorf("Error while saving report summary to outputs %s", reportSaveErr.Error())
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

			return exited, outputs, artifact, outputsV2, string(optimizationState), finalErr

		} else {
			outputs, err := fetchOutputVariables(outputFile, out, false) // nolint:govet
			if err != nil {
				return exited, nil, nil, nil, string(optimizationState), err
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
				return exited, outputs, artifact, outputsV2, string(optimizationState), err
			} else if len(r.OutputVars) > 0 {
				// only return err when output vars are expected
				if report.TestSummaryAsOutputEnabled(r.Envs) {
					return exited, summaryOutputs, artifact, summaryOutputsV2, string(optimizationState), err
				}
				return exited, outputs, artifact, nil, string(optimizationState), err
			}
			if len(summaryOutputsV2) != 0 && report.TestSummaryAsOutputEnabled(r.Envs) {
				return exited, outputs, artifact, summaryOutputsV2, string(optimizationState), nil
			}
			return exited, outputs, artifact, nil, string(optimizationState), nil
		}
	}
	if len(summaryOutputsV2) != 0 && report.TestSummaryAsOutputEnabled(r.Envs) {
		return exited, summaryOutputs, artifact, summaryOutputsV2, string(optimizationState), err
	}
	return exited, nil, artifact, nil, string(optimizationState), err
}
