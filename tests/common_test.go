package main_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

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
	return &runner{
		program: klock,
		args:    arg,
	}
}

func newKubectl(arg ...string) *runner {
	return &runner{
		program: kubectl,
		args:    arg,
	}
}

type runner struct {
	program string
	args    []string
	stdin   string
	dir     string
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
	cmd := exec.Command(r.program, r.args...)
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
	err := cmd.Run()
	return &result{
		stdout:     stdout.String(),
		stderr:     stderr.String(),
		err:        err,
		exitStatus: cmd.ProcessState.ExitCode(),
	}
}
