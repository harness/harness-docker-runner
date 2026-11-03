// Copyright 2026 Drone.IO Inc. All rights reserved.
// Use of this source code is governed by the Polyform License
// that can be found in the LICENSE file.

package docker

import (
	"reflect"
	"testing"

	"github.com/harness/harness-docker-runner/engine/spec"
)

// TestToNetConfig_NetworkAliasesAppended asserts user-provided network aliases
// are registered on the stage network alongside the computed step name
// hostname references keep resolving.
func TestToNetConfig_NetworkAliasesAppended(t *testing.T) {
	pcfg := &spec.PipelineConfig{}
	pcfg.Network.ID = "net-1"
	proc := &spec.Step{
		Name:           "grpA_grpB_sql",
		Network:        "net-1",
		NetworkAliases: []string{"sql", "db"},
	}

	cfg := toNetConfig(pcfg, proc)

	ep, ok := cfg.EndpointsConfig["net-1"]
	if !ok {
		t.Fatalf("expected endpoint on stage network net-1, got %+v", cfg.EndpointsConfig)
	}
	want := []string{"grpA_grpB_sql", "sql", "db"}
	if !reflect.DeepEqual(ep.Aliases, want) {
		t.Fatalf("aliases mismatch: got %v, want %v", ep.Aliases, want)
	}
}

// TestToNetConfig_NoNetworkAliases preserves prior behavior when the field is
// absent: only the computed step name is registered as an alias.
func TestToNetConfig_NoNetworkAliases(t *testing.T) {
	pcfg := &spec.PipelineConfig{}
	pcfg.Network.ID = "net-1"
	proc := &spec.Step{
		Name:    "grpA_grpB_sql",
		Network: "net-1",
	}

	cfg := toNetConfig(pcfg, proc)

	ep, ok := cfg.EndpointsConfig["net-1"]
	if !ok {
		t.Fatalf("expected endpoint on stage network net-1, got %+v", cfg.EndpointsConfig)
	}
	want := []string{"grpA_grpB_sql"}
	if !reflect.DeepEqual(ep.Aliases, want) {
		t.Fatalf("aliases mismatch: got %v, want %v", ep.Aliases, want)
	}
}

// TestToNetConfig_StepOffStageNetwork keeps the existing early return: a step
// not attached to the stage network gets no endpoint config at all, aliases or
// not.
func TestToNetConfig_StepOffStageNetwork(t *testing.T) {
	pcfg := &spec.PipelineConfig{}
	pcfg.Network.ID = "net-1"
	proc := &spec.Step{
		Name:           "grpA_grpB_sql",
		Network:        "other-net",
		NetworkAliases: []string{"sql"},
	}

	cfg := toNetConfig(pcfg, proc)
	if len(cfg.EndpointsConfig) != 0 {
		t.Fatalf("expected no endpoints for off-network step, got %+v", cfg.EndpointsConfig)
	}
}
