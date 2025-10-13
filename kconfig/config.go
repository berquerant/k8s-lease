package kconfig

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Build kubeconfig.
func Build(kubeconfig string) (*rest.Config, error) {
	return build(selectPath(kubeconfig))
}

func build(kubeconfig string) (*rest.Config, error) {
	c, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func selectPath(kubeconfig string) string {
	if x := kubeconfig; x != "" {
		return x
	}
	if x := os.Getenv("KUBECONFIG"); x != "" {
		return x
	}
	if d, err := os.UserHomeDir(); err == nil {
		return filepath.Join(d, ".kube", "config")
	}
	return ""
}
