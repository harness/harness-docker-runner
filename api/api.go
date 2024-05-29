// Copyright 2022 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package api

import (
	"github.com/harness/harness-docker-runner/engine/spec"
	leapi "github.com/harness/lite-engine/api"
)

type CommandExecutionStatus string

type Status string

const (
	Success      CommandExecutionStatus = "SUCCESS"
	Failure      CommandExecutionStatus = "FAILURE"
	RunningState CommandExecutionStatus = "RUNNING"
	Queued       CommandExecutionStatus = "QUEUED"
	Skipped      CommandExecutionStatus = "SKIPPED"
	Junit                               = leapi.Junit
)

type (
	RunTestConfig = leapi.RunTestConfig
	TestReport    = leapi.TestReport
	TIConfig      = leapi.TIConfig
)

type (
	HealthResponse struct {
		Version         string `json:"version"`
		DockerInstalled bool   `json:"docker_installed"`
		GitInstalled    bool   `json:"git_installed"`
		LiteEngineLog   string `json:"lite_engine_log"`
		OK              bool   `json:"ok"`
	}

	SetupRequest struct {
		ID                 string            `json:"id"` // stage runtime ID
		PoolID             string            `json:"pool_id"`
		Tags               map[string]string `json:"tags"`
		CorrelationID      string            `json:"correlation_id"`
		LogKey             string            `json:"log_key"`
		InfraType          string            `json:"infra_type"`
		SetupRequestConfig `json:"setup_request"`
	}

	SetupRequestConfig struct {
		Envs              map[string]string `json:"envs,omitempty"`
		Network           spec.Network      `json:"network"`
		Volumes           []*spec.Volume    `json:"volumes,omitempty"`
		Secrets           []string          `json:"secrets,omitempty"`
		LogConfig         LogConfig         `json:"log_config,omitempty"`
		TIConfig          TIConfig          `json:"ti_config,omitempty"`
		Files             []*spec.File      `json:"files,omitempty"`
		MountDockerSocket *bool             `json:"mount_docker_socket,omitempty"`
		CorrelationID     string            `json:"correlation_id"`
		LogKey            string            `json:"log_key"`
	}

	SetupResponse struct {
		IPAddress  string `json:"ip_address"`
		InstanceID string `json:"instance_id"`
	}

	DestroyRequest struct {
		ID string `json:"id"` // stage runtime ID
	}

	DebugRequest struct {
		StepID         string `json:"step_id"`
		Command        string `json:"command"`
		Last           bool   `json:"last"`
		StageRuntimeID string `json:"stage_runtime_id"`
	}

	DestroyResponse struct{}

	StartStepRequest struct {
		StageRuntimeID         string `json:"stage_runtime_id"`
		IPAddress              string `json:"ip_address"`
		PoolID                 string `json:"pool_id"`
		CorrelationID          string `json:"correlation_id"`
		StartStepRequestConfig `json:"start_step_request"`
	}

	StartStepRequestConfig struct {
		ID         string            `json:"id,omitempty"` // Unique identifier of step
		InfraType  string            `json:"infra_type"`
		Detach     bool              `json:"detach,omitempty"`
		Envs       map[string]string `json:"environment,omitempty"`
		Name       string            `json:"name,omitempty"`
		LogKey     string            `json:"log_key,omitempty"`
		LogDrone   bool              `json:"log_drone"`
		Secrets    []string          `json:"secrets,omitempty"`
		WorkingDir string            `json:"working_dir,omitempty"`
		Kind       StepType          `json:"kind,omitempty"`
		Run        RunConfig         `json:"run,omitempty"`
		RunTest    RunTestConfig     `json:"run_test,omitempty"`

		OutputVars        []string    `json:"output_vars,omitempty"`
		TestReport        TestReport  `json:"test_report,omitempty"`
		Timeout           int         `json:"timeout,omitempty"` // step timeout in seconds
		MountDockerSocket *bool       `json:"mount_docker_socket"`
		Outputs           []*OutputV2 `json:"outputs,omitempty"`

		// Valid only for steps running on docker container
		Auth         *spec.Auth           `json:"auth,omitempty"`
		CPUPeriod    int64                `json:"cpu_period,omitempty"`
		CPUQuota     int64                `json:"cpu_quota,omitempty"`
		CPUShares    int64                `json:"cpu_shares,omitempty"`
		CPUSet       []string             `json:"cpu_set,omitempty"`
		Devices      []*spec.VolumeDevice `json:"devices,omitempty"`
		DNS          []string             `json:"dns,omitempty"`
		DNSSearch    []string             `json:"dns_search,omitempty"`
		ExtraHosts   []string             `json:"extra_hosts,omitempty"`
		IgnoreStdout bool                 `json:"ignore_stderr,omitempty"`
		IgnoreStderr bool                 `json:"ignore_stdout,omitempty"`
		Image        string               `json:"image,omitempty"`
		Labels       map[string]string    `json:"labels,omitempty"`
		MemSwapLimit int64                `json:"memswap_limit,omitempty"`
		MemLimit     int64                `json:"mem_limit,omitempty"`
		Network      string               `json:"network,omitempty"`
		Networks     []string             `json:"networks,omitempty"`
		PortBindings map[string]string    `json:"port_bindings,omitempty"` // Host port to container port mapping
		Privileged   bool                 `json:"privileged,omitempty"`
		Pull         spec.PullPolicy      `json:"pull,omitempty"`
		ShmSize      int64                `json:"shm_size,omitempty"`
		User         string               `json:"user,omitempty"`
		Volumes      []*spec.VolumeMount  `json:"volumes,omitempty"`
		Files        []*spec.File         `json:"files,omitempty"`
	}

	OutputV2 struct {
		Key   string `json:"key,omitempty"`
		Value string `json:"value,omitempty"`
		Type  string `json:"type,omitempty"`
	}

	DelegateMetaInfo struct {
		ID       string `json:"id"`
		HostName string `json:"host_name"`
	}

	VMServiceStatus struct {
		ID           string `json:"identifier"`
		Name         string `json:"name"`
		Image        string `json:"image"`
		LogKey       string `json:"log_key"`
		Status       Status `json:"status"`
		ErrorMessage string `json:"error_message"`
	}

	StartStepResponse struct {
		ErrorMessage           string                 `json:"error_message"`
		IPAddress              string                 `json:"ip_address"`
		OutputVars             map[string]string      `json:"output_vars"`
		ServiceStatuses        []VMServiceStatus      `json:"service_statuses"`
		CommandExecutionStatus CommandExecutionStatus `json:"command_execution_status"`
		DelegateMetaInfo       DelegateMetaInfo       `json:"delegate_meta_info"`
	}

	PollStepRequest struct {
		ID string `json:"id,omitempty"`
	}

	PollStepResponse struct {
		Exited    bool              `json:"exited,omitempty"`
		ExitCode  int               `json:"exit_code,omitempty"`
		Error     string            `json:"error,omitempty"`
		OOMKilled bool              `json:"oom_killed,omitempty"`
		Outputs   map[string]string `json:"outputs,omitempty"`
		Artifact  []byte            `json:"artifact,omitempty"`
		OutputV2  []*OutputV2       `json:"outputV2,omitempty"`
	}

	StreamOutputRequest struct {
		ID     string `json:"id,omitempty"`
		Offset int    `json:"offset,omitempty"`
	}

	RunConfig struct {
		Command    []string `json:"commands,omitempty"`
		Entrypoint []string `json:"entrypoint,omitempty"`
	}

	LogConfig struct {
		AccountID      string `json:"account_id,omitempty"`
		IndirectUpload bool   `json:"indirect_upload,omitempty"` // Whether to directly upload via signed link or using log service
		URL            string `json:"url,omitempty"`
		Token          string `json:"token,omitempty"`
	}

	JunitReport struct {
		Paths []string `json:"paths,omitempty"`
	}
)
