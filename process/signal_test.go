package process_test

import (
	"syscall"
	"testing"

	"github.com/berquerant/k8s-lease/process"
	"github.com/stretchr/testify/assert"
)

func TestNewSignal(t *testing.T) {
	for _, tc := range []struct {
		s    string
		i    int
		want syscall.Signal
		ok   bool
	}{
		{
			s:    "INT",
			i:    2,
			want: syscall.SIGINT,
			ok:   true,
		},
		{
			s:    "TERM",
			i:    15,
			want: syscall.SIGTERM,
			ok:   true,
		},
		{
			s:    "15",
			i:    15,
			want: syscall.SIGTERM,
			ok:   true,
		},
		{
			s:  "UNKNOWN",
			i:  129,
			ok: false,
		},
	} {
		t.Run(tc.s, func(t *testing.T) {
			t.Run("string", func(t *testing.T) {
				got, ok := process.NewSignal(tc.s)
				if !tc.ok {
					assert.False(t, ok)
					return
				}
				assert.Equal(t, tc.want, got)
			})
			t.Run("int", func(t *testing.T) {
				got, ok := process.NewSignal(tc.i)
				if !tc.ok {
					assert.False(t, ok)
					return
				}
				assert.Equal(t, tc.want, got)
			})
		})
	}
}
