package executor

import (
	"fmt"
	"sync"

	"github.com/harness/lite-engine/engine"
	"github.com/harness/lite-engine/pipeline"
	"github.com/harness/lite-engine/pipeline/runtime"
)

var (
	executor *Executor
	once     sync.Once
)

//TODO:xun add mutex
// Executor maps stage runtime ID to the state of the stage
type Executor struct {
	m map[string]*StageData
	//TODO:xun add map to track output vars
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
	if _, ok := e.m[s]; !ok {
		err := fmt.Errorf("stage id %s does not exist, can not get stage info.", s)
		return nil, err
	}
	return e.m[s], nil
}

// Add maps the stage runtime ID to the stage data
func (e *Executor) Add(s string, sd *StageData) error {
	if _, ok := e.m[s]; ok {
		return fmt.Errorf("stage id %s already exist, can not add stage info again.", s)
	}
	e.m[s] = sd
	return nil
}

// Remove removes the stage runtime ID from the execution list
func (e *Executor) Remove(s string) (*StageData, error) {
	stageData, ok := e.m[s]
	if !ok {
		return nil, fmt.Errorf("stage id %s does not exist, can not remove stage info.", s)
	}
	delete(e.m, s)
	return stageData, nil
}

// StageData stores the engine and the pipeline state corresponding to a
// stage execution
type StageData struct {
<<<<<<< Updated upstream
	Engine       *engine.Engine
	State        *pipeline.State
	StepExecutor *runtime.StepExecutor
=======
	Engine        *engine.Engine
	State         *pipeline.State
	StepExecutors []*runtime.StepExecutor
	Network       string
>>>>>>> Stashed changes
}
