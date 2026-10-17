package process_test

import (
	"context"
	"testing"

	"github.com/berquerant/k8s-lease/lease"
	"github.com/berquerant/k8s-lease/process"
	"github.com/stretchr/testify/assert"
)

func TestProcessValidate(t *testing.T) {
	locker := &lease.Locker{}
	for _, tc := range []struct {
		title string
		proc  *process.Process
	}{
		{
			title: "locker is nil",
			proc:  process.NewProcess(nil, "echo", "hello"),
		},
		{
			title: "args is empty",
			proc:  &process.Process{},
		},
		{
			title: "program is empty",
			proc:  process.NewProcess(locker, ""),
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			err := tc.proc.Run(context.Background())
			assert.ErrorIs(t, err, process.ErrInvalidProcess)
		})
	}
}
