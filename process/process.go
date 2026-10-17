package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/berquerant/k8s-lease/lease"
)

func NewProcess(locker *lease.Locker, name string, arg ...string) *Process {
	return &Process{
		locker: locker,
		Args:   append([]string{name}, arg...),
	}
}

// Process is an external command executed under lock control.
type Process struct {
	locker       *lease.Locker
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	Args         []string
	CancelSignal os.Signal
	WaitDelay    time.Duration
}

var ErrInvalidProcess = errors.New("InvalidProcess")

func (p *Process) validate() error {
	if p.locker == nil {
		return fmt.Errorf("%w: locker is nil", ErrInvalidProcess)
	}
	if len(p.Args) == 0 {
		return fmt.Errorf("%w: args is nil", ErrInvalidProcess)
	}
	if p.Args[0] == "" {
		return fmt.Errorf("%w: program is empty", ErrInvalidProcess)
	}
	return nil
}

// Run starts the specified command and waits for it to complete with the lease lock.
func (p *Process) Run(ctx context.Context) error {
	if err := p.validate(); err != nil {
		return err
	}

	var (
		logger = p.locker.Logger(ctx)
		run    = func(ctx context.Context) error {
			cmd := exec.CommandContext(ctx, p.Args[0], p.Args[1:]...)
			cmd.Stdin = p.Stdin
			cmd.Stdout = p.Stdout
			cmd.Stderr = p.Stderr
			cmd.WaitDelay = p.WaitDelay
			if s := p.CancelSignal; s != syscall.Signal(0) {
				cmd.Cancel = func() error {
					sigstr := SignalIntoString(s)
					signum, _ := SignalIntoInt(s)
					logger.V(0).Info("process cancel", "signal", sigstr, "signum", signum, "waitDelay", cmd.WaitDelay)
					return cmd.Process.Signal(s)
				}
			}
			logger.V(0).Info("process start", "command", cmd.Args)
			err := cmd.Run()
			logger.V(0).Info("process end")
			return err
		}
	)

	if err := p.locker.LockAndRun(ctx, run); err != nil {
		logger.Error(err, "process LockAndRun")
		return fmt.Errorf("%w: locker=%s", err, p.locker)
	}
	return nil
}
