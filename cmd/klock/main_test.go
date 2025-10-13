package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/berquerant/k8s-lease/lease"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/labels"
)

func TestE2E(t *testing.T) {
	if !assert.Nil(t, build(), "should build klock binary") {
		return
	}
	if !assert.Nil(t, cleanupLeases()) {
		return
	}
	defer func() {
		_ = cleanupLeases()
	}()

	t.Run("serial", func(t *testing.T) {
		for _, tc := range []struct {
			title          string
			name           string
			launchDuration time.Duration
			k1SleepSecond  float64
			k2Wait         string
			k2FileExist    bool
		}{
			{
				title:          "should run",
				name:           "serial-should-run",
				launchDuration: time.Millisecond * 500,
				k1SleepSecond:  1.2,
				k2Wait:         "0",
				k2FileExist:    true,
			},
			{
				title:          "should run with long wait",
				name:           "serial-should-run-with-long-wait",
				launchDuration: time.Millisecond * 500,
				k1SleepSecond:  1.2,
				k2Wait:         "1h",
				k2FileExist:    true,
			},
			{
				title:          "should timed out",
				name:           "serial-should-timed-out",
				launchDuration: time.Millisecond * 500,
				k1SleepSecond:  2.0,
				k2Wait:         "1s",
				k2FileExist:    false,
			},
		} {
			t.Run(tc.title, func(t *testing.T) {
				const conflictExitCode = 5
				var (
					tmpd         = t.TempDir()
					k1File       = filepath.Join(tmpd, "k1")
					k2File       = filepath.Join(tmpd, "k2")
					k1Script     = filepath.Join(tmpd, "k1script.sh")
					k1ScriptData = fmt.Sprintf(`#!/bin/sh
set -ex
touch %s
sleep %f`, k1File, tc.k1SleepSecond)
					k1 = newKlock("-l", tc.name, "-i", tc.name+"-k1", "-E", strconv.Itoa(conflictExitCode), "--",
						"sh", k1Script)
					k2 = newKlock("-l", tc.name, "-i", tc.name+"-k2", "-E", strconv.Itoa(conflictExitCode), "-w", tc.k2Wait, "--",
						"touch", k2File)
					wg                 sync.WaitGroup
					k1Result, k2Result *result
				)
				if !assert.Nil(t, os.WriteFile(k1Script, []byte(k1ScriptData), 0750)) {
					return
				}

				wg.Add(2)
				go func() {
					k1Result = k1.run()
					wg.Done()
				}()
				time.Sleep(tc.launchDuration)
				go func() {
					k2Result = k2.run()
					wg.Done()
				}()
				wg.Wait()
				k1Result.assertSuccess(t)
				k1Stat, err := os.Stat(k1File)
				if !assert.Nil(t, err) {
					return
				}
				if !tc.k2FileExist {
					assert.NotNil(t, k2Result.err)
					assert.Equal(t, conflictExitCode, k2Result.exitStatus)
					_, err = os.Stat(k2File)
					assert.True(t, os.IsNotExist(err))
					return
				}
				k2Result.assertSuccess(t)
				k2Stat, err := os.Stat(k2File)
				if !assert.Nil(t, err) {
					return
				}
				const k2TouchTimeToleration = time.Millisecond * 500
				assert.True(t, k1Stat.ModTime().Add(time.Duration(tc.k1SleepSecond)*time.Second).Before(k2Stat.ModTime().Add(k2TouchTimeToleration)))
			})
		}
	})

	t.Run("onetime", func(t *testing.T) {
		t.Run("should run", func(t *testing.T) {
			const name = "onetime-should-run"
			r := newKlock("-l", name, "--", "echo", "ok").run()
			r.assertSuccess(t)
			assert.Equal(t, "ok\n", r.stdout)

			r = newKubectl("get", "lease", name).run()
			r.assertSuccess(t)
		})
		t.Run("should delete lease", func(t *testing.T) {
			const name = "onetime-should-delete-lease"
			r := newKlock("-l", name, "-u", "--", "echo", "ok").run()
			r.assertSuccess(t)
			assert.Equal(t, "ok\n", r.stdout)

			r = newKubectl("get", "lease", name).run()
			assert.NotNil(t, r.err)
			assert.Equal(t, 1, r.exitStatus)
			assert.Contains(t, r.stderr, fmt.Sprintf(`leases.coordination.k8s.io "%s" not found`, name))
		})
		t.Run("should read stdin", func(t *testing.T) {
			k := newKlock("-l", "onetime-should-read-stdin", "--", "grep", "key")
			k.stdin = `creamy
keyword
crispy
`
			r := k.run()
			r.assertSuccess(t)
			assert.Equal(t, "keyword\n", r.stdout)
		})
		t.Run("should add labels", func(t *testing.T) {
			const name = "onetime-should-add-labels"
			r := newKlock("-l", name, "--labels", "key1=value1,key2=value2", "--", "echo", "ok").run()
			r.assertSuccess(t)
			assert.Equal(t, "ok\n", r.stdout)

			r = newKubectl("get", "lease", name, "-o=jsonpath={.metadata.labels}").run()
			r.assertSuccess(t)
			got := map[string]string{}
			if !assert.Nil(t, json.Unmarshal([]byte(r.stdout), &got)) {
				return
			}
			want := map[string]string{
				"key1":                         "value1",
				"key2":                         "value2",
				"app.kubernetes.io/managed-by": "klock",
			}
			assert.Equal(t, want, got)
		})
	})
}

func build() error {
	cmd := exec.Command("make")
	cmd.Dir = "../.."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cleanupLeases() error {
	return newKubectl("delete", "lease", "-l", labels.SelectorFromSet(lease.CommonLabels()).String()).run().err
}

const (
	klock   = "../../dist/klock"
	kubectl = "../../hack/kubectl"
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
