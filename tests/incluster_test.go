package main_test

import (
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
)

func TestInCluster(t *testing.T) {
	if !assert.Nil(t, cleanupLeases()) {
		return
	}
	defer func() {
		_ = cleanupLeases()
	}()

	const (
		name                 = "test-incluster-run"
		namespace            = "default"
		localPath            = "/mnt/local-data" // from .cluster.yaml
		containerPath        = "/usr/local/dist"
		binary               = "klock-incluster-test" // from Makefile TEST_BIN
		manifestTemplatePath = "./manifest.yaml"
	)

	manifestTemplate, err := template.ParseFiles(manifestTemplatePath)
	if !assert.Nil(t, err) {
		return
	}

	var (
		manifestPath = filepath.Join(t.TempDir(), "test.yaml")
		cleanup      = func() error {
			return newKubectl("delete", "-f", manifestPath, "--ignore-not-found=true").run().err
		}
	)
	f, err := os.Create(manifestPath)
	if !assert.Nil(t, err) {
		return
	}
	if !assert.Nil(t, manifestTemplate.Execute(f, struct {
		Name          string
		Namespace     string
		LocalPath     string
		ContainerPath string
		Binary        string
	}{
		Name:          name,
		Namespace:     namespace,
		LocalPath:     localPath,
		ContainerPath: containerPath,
		Binary:        binary,
	})) {
		_ = f.Close()
		return
	}
	if !assert.Nil(t, f.Close()) {
		return
	}

	if !assert.Nil(t, cleanup()) {
		return
	}
	defer cleanup()
	if !assert.Nil(t, newKubectl("apply", "-f", manifestPath).run().err) {
		return
	}
	assert.Nil(t, newKubectl("wait", "--for=condition=complete", "job/"+name, "--timeout=60s").run().err)
	assert.Nil(t, newKubectl("logs", "job/"+name).run().err)
}
