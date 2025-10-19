package main_test

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSignal(t *testing.T) {
	if !assert.Nil(t, cleanupLeases()) {
		return
	}
	defer func() {
		_ = cleanupLeases()
	}()

	bin := filepath.Join(t.TempDir(), "signal")
	if !assert.Nil(t, newRunner("go", "build", "-o", bin, "./signal.go").run().err) {
		return
	}

	t.Run("testbin", func(t *testing.T) {
		t.Run("sigint", func(t *testing.T) {
			r := newRunner(bin, "-wait-signal")
			r.cancelDelay = time.Second
			g := r.run()
			assert.Zero(t, g.exitStatus)
			assert.Equal(t, "SIGINT\n", g.stdout)
		})
		t.Run("sigkill-wait", func(t *testing.T) {
			r := newRunner(bin, "-delay", "1s", "-wait-signal")
			r.cancelDelay = time.Second
			r.waitDelay = time.Millisecond * 500
			g := r.run()
			assert.Equal(t, -1, g.exitStatus)
			assert.Equal(t, "SIGINT\n", g.stdout)
		})
		t.Run("sigkill", func(t *testing.T) {
			r := newRunner(bin, "-delay", "5s")
			r.cancelDelay = time.Second
			r.waitDelay = time.Millisecond * 500
			g := r.run()
			assert.Equal(t, -1, g.exitStatus)
			assert.Empty(t, g.stdout)
		})
		t.Run("graceful", func(t *testing.T) {
			r := newRunner(bin, "-delay", "500ms")
			r.cancelDelay = time.Second
			r.waitDelay = time.Millisecond * 1000
			g := r.run()
			assert.Zero(t, g.exitStatus)
			assert.Empty(t, g.stdout)
		})
	})

	t.Run("exit", func(t *testing.T) {
		const name = "signal-exit"
		for i := range 3 {
			x := strconv.Itoa(i)
			t.Run("with "+x, func(t *testing.T) {
				r := newKlock("-l", name, "--", bin, "-exit", x).run()
				assert.Equal(t, i, r.exitStatus)
			})
		}
	})

	t.Run("cancel", func(t *testing.T) {
		const cancelDelay = time.Second
		// T=0, launch klock and the internal process (p)
		// T+cancelDelay, interrupt klock, send cancelSignal to p
		// If shutdownDelay > 0, p is still living
		// If killAfter > 0 and p is living, T+cancelDelay+killAfter, send SIGKILL to p
		for _, tc := range []struct {
			title         string
			name          string
			shutdownDelay string
			cancelSignal  string
			killAfter     string
			want          string
			exit          int
		}{
			{
				title:         "should SIGKILL",
				name:          "signal-sigkill",
				shutdownDelay: "10s",
				cancelSignal:  "INT",
				killAfter:     "2s",
				want:          "SIGINT\n",
				exit:          255,
			},
			{
				title:         "should not SIGKILL",
				name:          "signal-not-sigkill",
				shutdownDelay: "100ms",
				cancelSignal:  "INT",
				killAfter:     "1s",
				want:          "SIGINT\n",
				exit:          1,
			},
			{
				title:         "should send SIGINT",
				name:          "signal-sigint",
				shutdownDelay: "0",
				cancelSignal:  "INT",
				killAfter:     "0",
				want:          "SIGINT\n",
				exit:          1,
			},
			{
				title:         "should send SIGTERM",
				name:          "signal-sigterm",
				shutdownDelay: "0",
				cancelSignal:  "TERM",
				killAfter:     "0",
				want:          "SIGTERM\n",
				exit:          1,
			},
		} {
			t.Run(tc.title, func(t *testing.T) {
				k := newKlock("-l", tc.name, "-s", tc.cancelSignal, "--kill-after", tc.killAfter, "--", bin, "-wait-signal", "-delay", tc.shutdownDelay)
				k.cancelDelay = cancelDelay
				r := k.run()
				assert.Equal(t, tc.want, r.stdout)
				assert.Equal(t, tc.exit, r.exitStatus)
				assert.NotNil(t, r.err)
			})
		}
	})
}
