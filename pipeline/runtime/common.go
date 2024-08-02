// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/harness/godotenv/v3"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/harness/harness-docker-runner/logstream"
	"github.com/sirupsen/logrus"
)

const (
	ciEnablePluginOutputSecrets = "CI_ENABLE_PLUGIN_OUTPUT_SECRETS"
	trueValue                   = "true"
	outputDelimiterSpace        = " "
	outputDelimiterEquals       = "="
)

func getNudges() []logstream.Nudge {
	// <search-term> <resolution> <error-msg>
	return []logstream.Nudge{
		logstream.NewNudge("[Kk]illed", "Increase memory resources for the step", errors.New("out of memory")),
		logstream.NewNudge(".*git.* SSL certificate problem",
			"Set sslVerify to false in CI codebase properties", errors.New("SSL certificate error")),
		logstream.NewNudge("Cannot connect to the Docker daemon",
			"Setup dind if it's not running. If dind is running, privileged should be set to true",
			errors.New("could not connect to the docker daemon")),
	}
}

func getOutputVarCmd(entrypoint, outputVars []string, outputFile string, shouldEnableDotEnvSupport bool) string {
	isPsh := isPowershell(entrypoint)
	isPython := isPython(entrypoint)

	cmd := ""
	delimiter := outputDelimiterSpace
	if shouldEnableDotEnvSupport {
		delimiter = outputDelimiterEquals
	}
	if isPsh {
		cmd += fmt.Sprintf("\nNew-Item %s", outputFile)
	} else if isPython {
		cmd += "\nimport os\n"
	}
	for _, o := range outputVars {
		if isPsh {
			cmd += fmt.Sprintf("\n$val = \"%s%s$Env:%s\" \nAdd-Content -Path %s -Value $val", o, delimiter, o, outputFile)
		} else if isPython {
			cmd += fmt.Sprintf("with open('%s', 'a') as out_file:\n\tout_file.write('%s%s' + os.getenv('%s') + '\\n')\n", outputFile, o, delimiter, o)
		} else {
			cmd += fmt.Sprintf("\necho \"%s%s$%s\" >> %s", o, delimiter, o, outputFile)
		}
	}

	return cmd
}

func getOutputsCmd(entrypoint []string, outputVars []*api.OutputV2, outputFile string, shouldEnableDotEnvSupport bool) string {
	isPsh := isPowershell(entrypoint)
	isPython := isPython(entrypoint)

	cmd := ""
	delimiter := outputDelimiterSpace
	if shouldEnableDotEnvSupport {
		delimiter = outputDelimiterEquals
	}
	if isPsh {
		cmd += fmt.Sprintf("\nNew-Item %s", outputFile)
	} else if isPython {
		cmd += "\nimport os\n"
	}
	for _, o := range outputVars {
		if isPsh {
			cmd += fmt.Sprintf("\n$val = \"%s%s$Env:%s\" \nAdd-Content -Path %s -Value $val", o.Key, delimiter, o.Value, outputFile)
		} else if isPython {
			cmd += fmt.Sprintf("with open('%s', 'a') as out_file:\n\tout_file.write('%s%s' + os.getenv('%s') + '\\n')\n", outputFile, o.Key, delimiter, o.Value)
		} else {
			cmd += fmt.Sprintf("\necho \"%s%s$%s\" >> %s", o.Key, delimiter, o.Value, outputFile)
		}
	}

	return cmd
}

func fetchArtifactDataFromArtifactFile(artifactFile string, out io.Writer) ([]byte, error) {
	log := logrus.New()
	log.Out = out

	if _, err := os.Stat(artifactFile); errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	content, err := os.ReadFile(artifactFile)
	if err != nil {
		log.WithError(err).WithField("artifactFile", artifactFile).WithField("content", string(content)).Warnln("failed to read artifact file")
		return nil, err
	}
	return content, nil
}

func isPowershell(entrypoint []string) bool {
	if len(entrypoint) > 0 && (entrypoint[0] == "powershell" || entrypoint[0] == "pwsh") {
		return true
	}
	return false
}

func isPython(entrypoint []string) bool {
	if len(entrypoint) > 0 && (entrypoint[0] == "python3") {
		return true
	}
	return false
}

// Fetches map of env variable and value from OutputFile.
// OutputFile stores all env variable and value
func fetchOutputVariables(outputFile string, out io.Writer, isDotEnvFile bool) (map[string]string, error) {
	log := logrus.New()
	log.Out = out

	outputs := make(map[string]string)
	delimiter := outputDelimiterSpace
	if isDotEnvFile {
		delimiter = outputDelimiterEquals
	}

	// The output file maybe not exist - we don't consider that an error
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		return outputs, nil
	}

	f, err := os.Open(outputFile)
	if err != nil {
		log.WithError(err).WithField("outputFile", outputFile).Errorln("failed to open output file")
		return nil, err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		sa := strings.Split(line, delimiter)
		if len(sa) < 2 { // nolint:gomnd
			log.WithField("variable", sa[0]).Warnln("output variable does not exist")
		} else {
			outputs[sa[0]] = line[len(sa[0])+1:]
		}
	}
	if err := s.Err(); err != nil {
		log.WithError(err).Errorln("failed to create scanner from output file")
		return nil, err
	}
	return outputs, nil
}

func fetchExportedVarsFromEnvFile(envFile string, out io.Writer) (map[string]string, error) {
	log := logrus.New()
	log.Out = out

	defaultOutputs := make(map[string]string)
	if _, err := os.Stat(envFile); errors.Is(err, os.ErrNotExist) {
		return defaultOutputs, nil
	}

	var (
		env map[string]string
		err error
	)
	env, err = godotenv.Read(envFile)

	if err != nil {
		//fallback incase any parsing issue from godotenv package
		fallbackEnv, fallbackErr := fetchOutputVariables(envFile, out, true)
		if fallbackErr != nil {
			content, ferr := os.ReadFile(envFile)
			if ferr != nil {
				log.WithError(ferr).WithField("envFile", envFile).Warnln("Unable to read exported env file")
			}
			log.WithError(err).WithField("envFile", envFile).WithField("content", string(content)).Warnln("failed to read exported env file")
			if errors.Is(err, bufio.ErrTooLong) {
				err = fmt.Errorf("output variable length is more than %d bytes", bufio.MaxScanTokenSize)
			}
			return nil, err
		}
		return fallbackEnv, nil

	}
	return env, nil
}

func IsFeatureFlagEnabled(featureFlagName string, engine *engine.Engine, step *spec.Step) bool {
	if engine != nil && engine.IsFeatureFlagEnabled(featureFlagName) {
		return true
	}
	if step == nil {
		return false
	}
	val, ok := step.Envs[featureFlagName]
	return ok && val == trueValue
}

// checkStepSuccess checks if the step was successful based on the return values
func checkStepSuccess(state *runtime.State, err error) bool {
	if err == nil && state != nil && state.ExitCode == 0 && state.Exited {
		return true
	}
	return false
}
