package executor

import (
	"fmt"
	"sync"

	"github.com/harness/harness-docker-runner/engine"
	"github.com/harness/harness-docker-runner/pipeline"
	"github.com/harness/harness-docker-runner/pipeline/runtime"
)

var (
	executor *Executor
	once     sync.Once
)

// TODO:xun add mutex
// Executor maps stage runtime ID to the state of the stage
type Executor struct {
	m  map[string]*StageData
	mu sync.Mutex
}

// GetExecutor returns a singleton executor object used throughout the lifecycle
// of the runner
func GetExecutor() *Executor {
	once.Do(func() {
		executor = &Executor{
			m: make(map[string]*StageData),
		}
	})
	return executor
}

// Get returns the stage data if present, otherwise returns nil.
func (e *Executor) Get(s string) (*StageData, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.m[s]; !ok {
		err := fmt.Errorf("stage id %s does not exist, can not get stage info.", s)
		return nil, err
	}
	return e.m[s], nil
}

// Add maps the stage runtime ID to the stage data
func (e *Executor) Add(s string, sd *StageData) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.m[s]; ok {
		return fmt.Errorf("stage id %s already exist, can not add stage info again.", s)
	}
	e.m[s] = sd
	return nil
}

// Remove removes the stage runtime ID from the execution list
func (e *Executor) Remove(s string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.m[s]; !ok {
		return fmt.Errorf("could not remove mapping for id: %s as it doesn't exist", s)
	}
	delete(e.m, s)
	return nil
}

// StageData stores the engine and the pipeline state corresponding to a
// stage execution
type StageData struct {
	Engine       *engine.Engine
	State        *pipeline.State
	StepExecutor *runtime.StepExecutor
}
