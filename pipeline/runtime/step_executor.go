// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/drone/runner-go/pipeline/runtime"
	"github.com/harness/harness-docker-runner/api"
	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/engine/spec"
	"github.com/harness/harness-docker-runner/errors"
	"github.com/harness/harness-docker-runner/livelog"
	"github.com/harness/harness-docker-runner/logstream"
	tiCfg "github.com/harness/lite-engine/ti/config"
	"github.com/harness/ti-client/types"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
)

type ExecutionStatus int

type StepStatus struct {
	Status            ExecutionStatus
	State             *runtime.State
	StepErr           error
	Outputs           map[string]string
	Artifact          []byte
	OutputV2          []*api.OutputV2
	OptimizationState string
	Telemetry         *types.TelemetryData
	ErrorDetails      *api.ErrorDetails
}

const (
	NotStarted ExecutionStatus = iota
	Running
	Complete
)

type StepExecutor struct {
	engine     *engine.Engine
	mu         sync.Mutex
	stepStatus map[string]StepStatus
	stepLog    map[string]*StepLog
	stepWaitCh map[string][]chan StepStatus
}

func NewStepExecutor(engine *engine.Engine) *StepExecutor {
	return &StepExecutor{
		engine:     engine,
		mu:         sync.Mutex{},
		stepWaitCh: make(map[string][]chan StepStatus),
		stepLog:    make(map[string]*StepLog),
		stepStatus: make(map[string]StepStatus),
	}
}

func (e *StepExecutor) StartStep(ctx context.Context, r *api.StartStepRequest, secrets []string, client logstream.Client, tiConfig *tiCfg.Cfg, logConfig *api.LogConfig) error {
	if r.ID == "" {
		return &errors.BadRequestError{Msg: "ID needs to be set"}
	}

	e.mu.Lock()
	_, ok := e.stepStatus[r.ID]
	if ok {
		e.mu.Unlock()
		return nil
	}

	e.stepStatus[r.ID] = StepStatus{Status: Running}
	e.mu.Unlock()

	go func() {
		stdoutLogPath := getStdoutLogFilePath(r.ID)
		stderrLogPath := getStderrLogFilePath(r.ID)
		defer cleanupLogFiles(stdoutLogPath, stderrLogPath)

		state, outputs, artifact, outputV2, optimizationState, telemetry, stepErr := e.executeStep(r, secrets, client, tiConfig, logConfig)

		// Post annotations to Pipeline Service if step succeeded and feature is enabled
		ffEnabled := isAnnotationsEnabled(r.StartStepRequestConfig.Envs)
		if stepErr == nil && state != nil && state.ExitCode == 0 && ffEnabled {
			go e.postAnnotationsToPipeline(context.Background(), r)
		} else {
			logrus.Infoln("Annotations NOT posted - conditions not met")
		}

		exitCode := 0
		if state != nil {
			exitCode = state.ExitCode
		}

		var errorDetails *api.ErrorDetails
		if shouldCategorizeError(exitCode, stepErr, e.engine) {
			errorDetails = evaluateErrorCategorization(
				r.WorkingDir, stdoutLogPath, stderrLogPath,
				exitCode, r.ID, r.StageRuntimeID,
				r.StartStepRequestConfig.Envs, tiConfig, e.engine,
			)
		}

		status := StepStatus{Status: Complete, State: state, StepErr: stepErr, Outputs: outputs, Artifact: artifact, OutputV2: outputV2, OptimizationState: optimizationState, Telemetry: telemetry, ErrorDetails: errorDetails}
		e.mu.Lock()
		e.stepStatus[r.ID] = status
		channels := e.stepWaitCh[r.ID]
		e.mu.Unlock()

		for _, ch := range channels {
			ch <- status
		}
	}()
	return nil
}

func (e *StepExecutor) PollStep(ctx context.Context, r *api.PollStepRequest) (*api.PollStepResponse, error) {
	id := r.ID
	if r.ID == "" {
		return &api.PollStepResponse{}, &errors.BadRequestError{Msg: "ID needs to be set"}
	}

	e.mu.Lock()
	s, ok := e.stepStatus[id]
	if !ok {
		e.mu.Unlock()
		return &api.PollStepResponse{}, &errors.BadRequestError{Msg: "Step has not started"}
	}

	if s.Status == Complete {
		e.mu.Unlock()
		return convertStatus(s), nil
	}

	ch := make(chan StepStatus, 1)
	if _, ok := e.stepWaitCh[id]; !ok {
		e.stepWaitCh[id] = append(e.stepWaitCh[id], ch)
	} else {
		e.stepWaitCh[id] = []chan StepStatus{ch}
	}
	e.mu.Unlock()

	status := <-ch
	return convertStatus(status), nil
}

func (e *StepExecutor) StreamOutput(ctx context.Context, r *api.StreamOutputRequest) (oldOut []byte, newOut <-chan []byte, err error) {
	id := r.ID
	if id == "" {
		err = &errors.BadRequestError{Msg: "ID needs to be set"}
		return
	}

	var stepLog *StepLog

	// the runner will call this function just before the call to start step, so we wait a while for the step to start
	for ts := time.Now(); ; {
		e.mu.Lock()
		stepLog = e.stepLog[id]
		e.mu.Unlock()

		if stepLog != nil {
			break
		}

		const timeoutDelay = 5 * time.Second
		if time.Since(ts) >= timeoutDelay {
			err = &errors.BadRequestError{Msg: "Step has not started"}
			return
		}

		const retryDelay = 100 * time.Millisecond
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			err = ctx.Err()
			return
		}
	}

	// subscribe to new data messages, and unsubscribe when the request context finished or when the step is done
	chData := make(chan []byte)
	oldOut, err = stepLog.Subscribe(chData, r.Offset)
	if err != nil {
		return
	}

	go func() {
		select {
		case <-ctx.Done():
			// the api request has finished/aborted
		case <-stepLog.Done():
			// the step has finished
		}
		close(chData)
		stepLog.Unsubscribe(chData)
	}()

	newOut = chData

	return //nolint:nakedret
}

func (e *StepExecutor) executeStepDrone(r *api.StartStepRequest, tiConfig *tiCfg.Cfg) (*runtime.State, error) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if r.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Second*time.Duration(r.Timeout))
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	stepLog := NewStepLog(ctx) // step output will terminate when the ctx is canceled

	logr := logrus.
		WithField("id", r.ID).
		WithField("step", r.Name)

	e.mu.Lock()
	e.stepLog[r.ID] = stepLog
	e.mu.Unlock()

	runStep := func() (*runtime.State, error) {
		defer cancel()

		r.Kind = api.Run // only this kind is supported

		exited, _, _, _, _, _, err := e.run(ctx, e.engine, r, stepLog, tiConfig, nil)
		if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
			logr.WithError(err).Warnln("step execution canceled")
			return nil, ctx.Err()
		}
		if err != nil {
			logr.WithError(err).Warnln("step execution failed")
			return nil, err
		}

		if exited != nil {
			if exited.OOMKilled {
				logr.Infoln("step received oom kill")
			} else {
				logr.WithField("exitCode", exited.ExitCode).Infoln("step terminated")
			}
		}

		return exited, nil
	}

	// if the step is configured as a daemon, it is detached
	// from the main process and executed separately.
	if r.Detach {
		go runStep() // nolint:errcheck
		return &runtime.State{Exited: false}, nil
	}

	return runStep()
}

func (e *StepExecutor) executeStep(r *api.StartStepRequest, secrets []string, client logstream.Client, tiConfig *tiCfg.Cfg, logConfig *api.LogConfig) (*runtime.State, map[string]string, []byte, []*api.OutputV2, string, *types.TelemetryData, error) {
	if r.LogDrone {
		state, err := e.executeStepDrone(r, tiConfig)
		return state, nil, nil, nil, "", nil, err
	}

	wc := livelog.New(client, r.LogKey, r.Name, getNudges(), logConfig.TrimNewLineSuffix)
	wr := logstream.NewReplacer(wc, secrets)
	err := wr.Open() // nolint:errcheck
	if err != nil {
		logrus.WithError(err).WithField("key", r.LogKey).Errorln("could not open log stream")
	}

	// if the step is configured as a daemon, it is detached
	// from the main process and executed separately.
	if r.Detach {
		go func() {
			ctx := context.Background()
			var cancel context.CancelFunc
			if r.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Second*time.Duration(r.Timeout))
				defer cancel()
			}
			e.run(ctx, e.engine, r, wr, tiConfig, nil) // nolint:errcheck
			wc.Close()
		}()
		return &runtime.State{Exited: false}, nil, nil, nil, "", nil, nil
	}

	var result error

	ctx := context.Background()
	var cancel context.CancelFunc
	if r.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Second*time.Duration(r.Timeout))
		defer cancel()
	}

	// Create per-step stdout/stderr capture files for error categorization.
	// Files persist after this function returns so the caller can pass them to hcli.
	var capture *spec.OutputCapture
	if IsFeatureFlagEnabled(errorCategorizationFF, e.engine, nil) {
		ensureLogDir()
		stdoutFile, serr := os.Create(getStdoutLogFilePath(r.ID))
		stderrFile, eerr := os.Create(getStderrLogFilePath(r.ID))
		if serr == nil && eerr == nil {
			defer stdoutFile.Close()
			defer stderrFile.Close()
			capture = &spec.OutputCapture{Stdout: stdoutFile, Stderr: stderrFile}
		} else {
			if serr != nil {
				logrus.WithError(serr).Warnln("failed to create stdout capture file for error categorization")
			}
			if eerr != nil {
				logrus.WithError(eerr).Warnln("failed to create stderr capture file for error categorization")
			}
			if stdoutFile != nil {
				stdoutFile.Close()
			}
			if stderrFile != nil {
				stderrFile.Close()
			}
		}
	}

	exited, outputs, artifact, outputV2, optimizationState, telemetry, err := e.run(ctx, e.engine, r, wr, tiConfig, capture)
	if err != nil {
		result = multierror.Append(result, err)
	}

	// Eager cleanup on success -- files not needed for categorization.
	// The deferred safety net in StartStep handles the double-cleanup case silently.
	if capture != nil && err == nil && exited != nil && exited.ExitCode == 0 {
		cleanupLogFiles(getStdoutLogFilePath(r.ID), getStderrLogFilePath(r.ID))
	}

	// close the stream. If the session is a remote session, the
	// full log buffer is uploaded to the remote server.
	if closeErr := wc.Close(); closeErr != nil {
		if IsFeatureFlagEnabled(ciLogServiceResilience, e.engine, nil) && exited != nil && exited.ExitCode == 0 && err == nil {
			logrus.WithError(closeErr).WithField("key", r.LogKey).
				Warnln("log close/upload failed but step succeeded, ignoring")
		} else {
			result = multierror.Append(result, closeErr)
		}
	}

	// if the context was canceled and returns a canceled or
	// DeadlineExceeded error this indicates the step was timed out.
	switch ctx.Err() {
	case context.Canceled, context.DeadlineExceeded:
		return nil, nil, nil, nil, "", telemetry, ctx.Err()
	}

	if exited != nil {
		if exited.ExitCode != 0 {
			if wc.Error() != nil {
				result = multierror.Append(result, err)
			}
		}

		if exited.OOMKilled {
			logrus.WithField("id", r.ID).Infoln("received oom kill.")
		} else {
			logrus.WithField("id", r.ID).Infof("received exit code %d\n", exited.ExitCode)
		}
	}
	return exited, outputs, artifact, outputV2, optimizationState, telemetry, result
}

func (e *StepExecutor) run(ctx context.Context, engine *engine.Engine, r *api.StartStepRequest, out io.Writer, tiConfig *tiCfg.Cfg, capture *spec.OutputCapture) (
	*runtime.State, map[string]string, []byte, []*api.OutputV2, string, *types.TelemetryData, error) {
	if r.Kind == api.Run {
		return executeRunStep(ctx, engine, r, out, tiConfig, capture)
	}
	if r.Kind == api.RunTestsV2 {
		return executeRunTestsV2Step(ctx, engine, r, out, tiConfig, capture)
	}
	return executeRunTestStep(ctx, engine, r, out, tiConfig, capture)
}

func convertStatus(status StepStatus) *api.PollStepResponse {
	r := &api.PollStepResponse{
		Exited:            true,
		Outputs:           status.Outputs,
		Artifact:          status.Artifact,
		OutputV2:          status.OutputV2,
		OptimizationState: status.OptimizationState,
		Telemetry:         status.Telemetry,
		ErrorDetails:      status.ErrorDetails,
	}

	stepErr := status.StepErr

	if status.State != nil {
		r.Exited = status.State.Exited
		r.OOMKilled = status.State.OOMKilled
		r.ExitCode = status.State.ExitCode
		if status.State.OOMKilled {
			stepErr = multierror.Append(stepErr, fmt.Errorf("oom killed"))
		} else if status.State.ExitCode != 0 {
			stepErr = multierror.Append(stepErr, fmt.Errorf("exit status %d", status.State.ExitCode))
		}
	}

	if status.StepErr != nil {
		r.ExitCode = 255
	}

	if stepErr != nil {
		r.Error = stepErr.Error()
	}
	return r
}
