package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	"al.essio.dev/pkg/shellescape"
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
	locker *lease.Locker
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Args   []string
}

var ErrInvalidProcess = errors.New("InvalidProcess")

func (p *Process) Validate() error {
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

func (p *Process) quotedArgs() []string {
	xs := make([]string, len(p.Args))
	for i, a := range p.Args {
		xs[i] = shellescape.Quote(a)
	}
	return xs
}

// Run starts the specified command and waits for it to complete with the lease lock.
//
// Run supports the same options as lease.Locker.LockAndRun.
func (p *Process) Run(ctx context.Context, opt ...lease.ConfigOption) error {
	if err := p.Validate(); err != nil {
		return err
	}

	var (
		logger = p.locker.Logger()
		args   = p.quotedArgs()
		run    = func(ctx context.Context) error {
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			cmd.Stdin = p.Stdin
			cmd.Stdout = p.Stdout
			cmd.Stderr = p.Stderr
			logger.Info("[Process] Start", slog.String("command", strings.Join(cmd.Args, " ")))
			err := cmd.Run()
			logger.Info("[Process] End")
			return err
		}
	)

	var (
		cmdErr  error
		lockErr error
	)
	if lockErr = p.locker.LockAndRun(ctx, func(ctx context.Context) {
		cmdErr = run(ctx)
	}, opt...); lockErr != nil {
		logger.Error("[Process] LockAndRun", slog.String("error", lockErr.Error()))
	}
	if err := errors.Join(cmdErr, lockErr); err != nil {
		return fmt.Errorf("%w: locker=%s", err, p.locker)
	}
	return nil
}
