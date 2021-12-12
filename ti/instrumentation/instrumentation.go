package instrumentation

import (
	"context"
	"fmt"
	"io"
	"runtime"

	"github.com/sirupsen/logrus"

	"github.com/harness/lite-engine/api"
	"github.com/harness/lite-engine/internal/filesystem"
	"github.com/harness/lite-engine/pipeline"
	"github.com/harness/lite-engine/ti"
	"github.com/harness/lite-engine/ti/instrumentation/csharp"
	"github.com/harness/lite-engine/ti/instrumentation/java"
)

func GetCmd(ctx context.Context, config *api.RunTestConfig, stepID, workspace string, out io.Writer) (string, error) {
	fs := filesystem.New()
	tmpFilePath := pipeline.SharedVolPath
	log := logrus.New()
	log.Out = out

	// Get the tests that need to be run if we are running selected tests
	var selection ti.SelectTestsResp

	files, err := getChangedFiles(ctx, workspace, log)
	if err != nil {
		return "", err
	}

	runOnlySelectedTests := config.RunOnlySelectedTests

	isManual := isManualExecution()
	if len(files) == 0 {
		log.Errorln("unable to get changed files list")
		runOnlySelectedTests = false // run all the tests if we could not find changed files list correctly
	}
	if isManual {
		log.Infoln("detected manual execution - for intelligence to be configured, a PR must be raised. Running all the tests.")
		runOnlySelectedTests = false // run all the tests if it is a manual execution
	}
	selection, err = selectTests(ctx, workspace, files, runOnlySelectedTests, stepID, fs)
	if err != nil {
		log.WithError(err).Errorln("there was some issue in trying to intelligently figure out tests to run. Running all the tests")
		runOnlySelectedTests = false // run all the tests if an error was encountered
	} else if !valid(selection.Tests) { // This shouldn't happen
		log.Warnln("test intelligence did not return suitable tests")
		runOnlySelectedTests = false // TI did not return suitable tests
	} else if selection.SelectAll {
		log.Infoln("intelligently determined to run all the tests")
		runOnlySelectedTests = false // TI selected all the tests to be run
	} else {
		log.Infoln(fmt.Sprintf("intelligently running tests: %s", selection.Tests))
	}

	var runner TestRunner
	switch config.Language {
	case "java":
		switch config.BuildTool {
		case "maven":
			runner = java.NewMavenRunner(log, fs)
		case "gradle":
			runner = java.NewGradleRunner(log, fs)
		case "bazel":
			runner = java.NewBazelRunner(log, fs)
		default:
			return "", fmt.Errorf("build tool: %s is not supported for Java", config.BuildTool)
		}
	case "csharp":
		{
			switch config.BuildTool {
			case "dotnet":
				runner = csharp.NewDotnetRunner(log, fs)
			default:
				return "", fmt.Errorf("build tool: %s is not supported for Csharp", config.BuildTool)
			}
		}
	default:
		return "", fmt.Errorf("language %s is not suported", config.Language)
	}

	// Install agent artifacts if not present
	// This should also return the absolute path to the agent artifacts. This value will get passed in GetCmd()
	// and the implementation can handle that.
	artifactDir, err := installAgents(ctx, tmpFilePath, config.Language, runtime.GOOS, runtime.GOARCH, config.BuildTool, fs)
	if err != nil {
		return "", err
	}

	// Create the config file required for instrumentation
	iniFilePath, err := createConfigFile(runner, config.Packages, config.TestAnnotations, config.Language, workspace, tmpFilePath, fs, log)
	if err != nil {
		return "", err
	}
	// agentArg := fmt.Sprintf(java.AgentArg, tmpFilePath, iniFilePath)

	testCmd, err := runner.GetCmd(ctx, selection.Tests, config.Args, iniFilePath, artifactDir, isManual, !runOnlySelectedTests)
	if err != nil {
		return "", err
	}

	// TMPDIR needs to be set for some build tools like bazel
	command := fmt.Sprintf("%s\n%s\n%s", config.PreCommand, testCmd, config.PostCommand)

	return command, nil
}
