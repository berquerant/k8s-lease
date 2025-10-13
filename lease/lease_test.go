package lease_test

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/berquerant/k8s-lease/lease"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8slabels "k8s.io/apimachinery/pkg/labels"
)

type sleeper struct {
	name       string
	duration   time.Duration
	called     bool
	calledTime time.Time
	canceled   bool
}

func (s *sleeper) sleep(ctx context.Context) {
	logger := slog.With(slog.String("name", s.name))
	logger.Debug("Sleeper: Start", slog.String("duration", s.duration.String()))
	s.called = true
	s.calledTime = time.Now()
	select {
	case <-ctx.Done():
		s.canceled = true
	case <-time.After(s.duration):
	}
	logger.Debug("Sleeper: End")
}

func newSleeper(name string, duration time.Duration) *sleeper {
	return &sleeper{
		name:     name,
		duration: duration,
	}
}

var _ = Describe("Locker", func() {
	Context("Labels", func() {
		It("should have the labels", func() {
			const name = "labels-should-have-the-labels"
			s := newSleeper(name, time.Millisecond*200)
			locker, err := lease.NewLocker(namespace, name, name+"-id", clientIface, lease.WithLabels(map[string]string{
				"additional": "label",
			}))
			Expect(err).To(Succeed())
			Expect(locker.LockAndRun(ctx, s.sleep)).To(Succeed())
			Expect(s.called).To(BeTrue())
			Expect(s.canceled).To(BeFalse())
			x, err := getLease(ctx, name)
			Expect(err).To(Succeed())
			labs := k8slabels.Set(x.GetLabels())
			Expect(labs.Get("app.kubernetes.io/managed-by")).To(Equal("k8s-lease-klock"))
			Expect(labs.Get("additional")).To(Equal("label"))
		})
	})

	Context("OneTime", func() {
		It("should be cancellable", func() {
			const name = "onetime-cancel"
			s := newSleeper(name, time.Minute)
			locker, err := lease.NewLocker(namespace, name, name+"-id", clientIface)
			Expect(err).To(Succeed())
			ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*300)
			defer cancel()
			// no error because f was successfully called
			Expect(locker.LockAndRun(ctx, s.sleep)).To(Succeed())
			Expect(s.called).To(BeTrue())
			Expect(s.canceled).To(BeTrue())
		})

		for _, tc := range []struct {
			title          string
			name           string
			sleepDuration  time.Duration
			cancelDuration time.Duration
			shouldCanceled bool
		}{
			{
				title:          "should call cleanup function",
				name:           "onetime-call-cleanup",
				sleepDuration:  time.Millisecond * 300,
				cancelDuration: time.Second,
				shouldCanceled: false,
			},
			{
				title:          "should call cleanup function even if canceled",
				name:           "onetime-call-cleanup-canceled",
				sleepDuration:  time.Millisecond * 500,
				cancelDuration: time.Millisecond * 200,
				shouldCanceled: true,
			},
		} {
			It(tc.title, func() {
				s := newSleeper(tc.name, tc.sleepDuration)
				u := newSleeper(tc.name+"-cleanup-function", time.Millisecond*100)
				locker, err := lease.NewLocker(namespace, tc.name, tc.name+"-id", clientIface)
				Expect(err).To(Succeed())
				ctx, cancel := context.WithTimeout(context.TODO(), tc.cancelDuration)
				defer cancel()
				Expect(locker.LockAndRun(ctx, s.sleep, lease.WithCleanup(func() { u.sleep(context.TODO()) }))).To(Succeed())
				Expect(s.called).To(BeTrue())
				Expect(s.canceled).To(Equal(tc.shouldCanceled))
				Expect(u.called).To(BeTrue())
				Expect(u.canceled).To(BeFalse())
			})
		}

		for _, tc := range []struct {
			title   string
			name    string
			cleanup bool
		}{
			{
				title:   "should run",
				name:    "ontime-run",
				cleanup: false,
			},
			{
				title:   "should run and remove the lease",
				name:    "onetime-run-cleanup-lease",
				cleanup: true,
			},
		} {
			It(tc.title, func() {
				s := newSleeper(tc.name, time.Millisecond*200)
				locker, err := lease.NewLocker(namespace, tc.name, tc.name+"-id", clientIface, lease.WithCleanupLease(tc.cleanup))
				Expect(err).To(Succeed())
				Expect(locker.LockAndRun(ctx, s.sleep)).To(Succeed())
				Expect(s.called).To(BeTrue())
				Expect(s.canceled).To(BeFalse())
				x, err := getLease(ctx, tc.name)
				if tc.cleanup {
					Expect(k8serrors.IsNotFound(err)).To(BeTrue())
					return
				}
				Expect(err).To(Succeed())
				Expect(tc.name).To(Equal(x.Name))
			})
		}
	})

	Context("Timeout", func() {
		for _, tc := range []struct {
			title          string
			name           string
			sleepDuration1 time.Duration
			sleepDuration2 time.Duration
			launchDelay    time.Duration
			electTimeout   time.Duration
			timedout       bool
		}{
			{
				title:          "should be canceled when the leader election timed out",
				name:           "parallel-leader-election-timeout",
				sleepDuration1: time.Millisecond * 800,
				sleepDuration2: time.Millisecond * 200,
				launchDelay:    time.Millisecond * 100,
				electTimeout:   time.Millisecond * 200,
				timedout:       true,
			},
			{
				title:          "should be continued when the leader election succeeded",
				name:           "parallel-leader-election-succeeded",
				sleepDuration1: time.Millisecond * 500,
				sleepDuration2: time.Millisecond * 200,
				launchDelay:    time.Millisecond * 100,
				electTimeout:   time.Second * 20,
				timedout:       false,
			},
		} {
			It(tc.title, func() {
				var (
					id1 = tc.name + "-id1"
					id2 = tc.name + "-id2"
					s1  = newSleeper(id1, tc.sleepDuration1)
					s2  = newSleeper(id2, tc.sleepDuration2)
				)
				locker1, err := lease.NewLocker(namespace, tc.name, id1, clientIface)
				Expect(err).To(Succeed())
				locker2, err := lease.NewLocker(namespace, tc.name, id2, clientIface)
				Expect(err).To(Succeed())

				var (
					wg         sync.WaitGroup
					err1, err2 error
				)
				wg.Add(2)
				// T=0, launch locker1
				go func() {
					err1 = locker1.LockAndRun(ctx, s1.sleep)
					wg.Done()
				}()
				time.Sleep(tc.launchDelay)
				// T+launchDelay, launch locker2
				go func() {
					err2 = locker2.LockAndRun(ctx, s2.sleep, lease.WithLeaderElectTimeout(tc.electTimeout))
					wg.Done()
				}()
				// T+sleepDuration1, locker1 should be completed
				// If T+sleepDuration1 > electTimeout, locker2 will abort
				wg.Wait()
				Expect(err1).To(Succeed())
				Expect(s1.called).To(BeTrue())
				Expect(s1.canceled).To(BeFalse())
				if tc.timedout {
					Expect(err2).To(MatchError(lease.ErrElectTimedOut))
				} else {
					Expect(err2).To(Succeed())
				}
				Expect(s2.called).To(Equal(!tc.timedout))
				Expect(s2.canceled).To(BeFalse())
			})
		}
	})

	Context("Parallel", func() {
		It("should be cancellable", func() {
			const (
				name             = "parallel-should-be-cancellable"
				timeoutDuration1 = time.Second * 3
				timeoutDuration2 = time.Second
				launchDelay      = time.Millisecond * 500
				sleepDuration1   = time.Minute
				sleepDuration2   = time.Second
			)
			var (
				id1 = name + "-id1"
				id2 = name + "-id2"
				s1  = newSleeper(id1, sleepDuration1)
				s2  = newSleeper(id2, sleepDuration2)
			)
			locker1, err := lease.NewLocker(namespace, name, id1, clientIface)
			Expect(err).To(Succeed())
			locker2, err := lease.NewLocker(namespace, name, id2, clientIface)
			Expect(err).To(Succeed())

			ctx1, cancel := context.WithTimeout(context.TODO(), timeoutDuration1)
			defer cancel()
			ctx2, cancel := context.WithTimeout(ctx1, timeoutDuration2)
			defer cancel()
			var (
				wg         sync.WaitGroup
				err1, err2 error
			)
			wg.Add(2)
			// T=0, launch locker1
			go func() {
				err1 = locker1.LockAndRun(ctx1, s1.sleep)
				wg.Done()
			}()
			time.Sleep(launchDelay)
			// T+launchDelay, launch locker2
			go func() {
				err2 = locker2.LockAndRun(ctx2, s2.sleep)
				wg.Done()
			}()
			// T+timeoutDuration, locker1 should be canceled because sleepDuration1 > timeoutDuration1
			// T+timeoutDuration, locker2 should be canceled because sleepDuration1 > timeoutDuration2 + launchDelay
			wg.Wait()
			// locker1 should succeed because s1.sleep was successfully called
			Expect(err1).To(Succeed())
			Expect(s1.called).To(BeTrue())
			Expect(s1.canceled).To(BeTrue())
			Expect(err2).To(MatchError(context.DeadlineExceeded))
			Expect(s2.called).To(BeFalse())
			Expect(s2.canceled).To(BeFalse())
		})

		for _, tc := range []struct {
			title          string
			name           string
			sleepDuration1 time.Duration
			sleepDuration2 time.Duration
			launchDelay    time.Duration
		}{
			{
				title:          "should execute sequentially after acquiring the lease",
				name:           "parallel-run-1",
				sleepDuration1: time.Millisecond * 700,
				sleepDuration2: time.Millisecond * 200,
				launchDelay:    time.Millisecond * 1000,
			},
			{
				title:          "should execute sequentially after acquiring the lease with heavy job",
				name:           "parallel-run-2",
				sleepDuration1: time.Millisecond * 2500,
				sleepDuration2: time.Millisecond * 200,
				launchDelay:    time.Millisecond * 1000,
			},
		} {
			It(tc.title, func() {
				var (
					id1 = tc.name + "-id1"
					id2 = tc.name + "-id2"
					s1  = newSleeper(id1, tc.sleepDuration1)
					s2  = newSleeper(id2, tc.sleepDuration2)
				)
				locker1, err := lease.NewLocker(namespace, tc.name, id1, clientIface)
				Expect(err).To(Succeed())
				locker2, err := lease.NewLocker(namespace, tc.name, id2, clientIface)
				Expect(err).To(Succeed())

				var (
					wg         sync.WaitGroup
					err1, err2 error
				)
				wg.Add(2)
				// T=0, launch locker1
				go func() {
					err1 = locker1.LockAndRun(ctx, s1.sleep)
					wg.Done()
				}()
				time.Sleep(tc.launchDelay)
				// T+launchDelay, launch locker2
				go func() {
					err2 = locker2.LockAndRun(ctx, s2.sleep)
					wg.Done()
				}()
				// T+sleepDuration1, locker1 should be completed
				// T+sleepDuration1+some duration, locker2 should be started
				wg.Wait()
				Expect(err1).To(Succeed())
				Expect(s1.called).To(BeTrue())
				Expect(s1.canceled).To(BeFalse())
				Expect(err2).To(Succeed())
				Expect(s2.called).To(BeTrue())
				Expect(s2.canceled).To(BeFalse())
				const s2StartTimeToleration = time.Millisecond * 500
				Expect(s1.calledTime.Add(s1.duration).Unix()).To(BeNumerically("<=", s2.calledTime.Add(s2StartTimeToleration).Unix()))
			})
		}
	})
})
