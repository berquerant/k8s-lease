package main_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/berquerant/k8s-lease/lease"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/labels"
)

func cleanupLeases() error {
	return newKubectl("delete", "lease", "-l", labels.SelectorFromSet(lease.CommonLabels()).String()).run().err
}

const (
	klock   = "../bin/klock"
	kubectl = "../hack/kubectl"
)

func newKlock(arg ...string) *runner {
	args := append([]string{"-v=2"}, arg...)
	return newRunner(klock, args...)
}

func newKubectl(arg ...string) *runner {
	return newRunner(kubectl, arg...)
}

func newRunner(program string, arg ...string) *runner {
	return &runner{
		program: program,
		args:    arg,
	}
}

type runner struct {
	program     string
	args        []string
	stdin       string
	dir         string
	cancelDelay time.Duration
	waitDelay   time.Duration
}

type result struct {
	stdout     string
	stderr     string
	exitStatus int
	err        error
}

func (r *result) assertSuccess(t *testing.T) {
	assert.Nil(t, r.err)
	assert.Zero(t, r.exitStatus)
}

func (r *runner) run() *result {
	cancelDelay := r.cancelDelay
	if cancelDelay == 0 {
		cancelDelay = time.Hour
	}
	ctx, cancel := context.WithTimeout(context.TODO(), cancelDelay)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.program, r.args...)
	if x := r.dir; x != "" {
		cmd.Dir = x
	}
	fmt.Fprintf(os.Stderr, "[runner.run] %v\n", cmd.Args)
	if s := r.stdin; s != "" {
		cmd.Stdin = bytes.NewBufferString(s)
	}
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGINT)
	}
	cmd.WaitDelay = r.waitDelay
	err := cmd.Run()
	return &result{
		stdout:     stdout.String(),
		stderr:     stderr.String(),
		err:        err,
		exitStatus: cmd.ProcessState.ExitCode(),
	}
}
