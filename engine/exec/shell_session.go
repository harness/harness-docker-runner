package exec

import (
	"io"
	"os/exec"
	"sync"
)

// ShellSession struct to hold the shell command and pipes
type ShellSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr io.ReadCloser
	mutex  sync.Mutex
	errors chan error
}

func NewShellSession(cmd *exec.Cmd) (*ShellSession, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	session := &ShellSession{
		cmd:    cmd,
		stdin:  stdin,
		mutex:  sync.Mutex{},
		errors: make(chan error, 1),
	}
	return session, nil
}

func (ss *ShellSession) Add(key, command string) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	ss.stdin.Write([]byte(command + "\n"))
}

func (ss *ShellSession) Wait(key string) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	ss.stdin.Close()
	err := ss.cmd.Wait()
	ss.errors <- err
}

var shellContextState *ShellContext

type ShellContext struct {
	ctx map[string]*ShellSession
}

func NewShellContext(key string, ss *ShellSession) {
	shellContextState = &ShellContext{
		ctx: map[string]*ShellSession{
			key: ss,
		},
	}
}

func GetShellSessionState(key string) *ShellSession {
	return shellContextState.ctx[key]
}
