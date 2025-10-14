package lease

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/utils/ptr"
)

var (
	ErrInvalidLocker = errors.New("InvalidLocker")
	ErrElectTimedOut = errors.New("ElectTimedOut")
)

//go:generate go tool goconfig -field "Labels labels.Set|CleanupLease bool|Cleanup func()|LeaderElectTimeout time.Duration" -option -output config_generated.go

// NewLocker creates the new Locker instance.
//
//   - namespace: the namespace of a lease
//   - name: the name of a lease
//   - id: the id of a lease holder
//   - client: the leases client
//
// Available options:
//
//   - WithLabels: the additional labels of a lease
//   - WithCleanupLease: if true, delete the created lease after processing (default: false)
func NewLocker(
	namespace, name, id string,
	client coordinationv1client.LeasesGetter,
	opt ...ConfigOption,
) (*Locker, error) {
	if namespace == "" {
		return nil, fmt.Errorf("%w: namespace is empty", ErrInvalidLocker)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: name is empty", ErrInvalidLocker)
	}
	if id == "" {
		return nil, fmt.Errorf("%w: id is empty", ErrInvalidLocker)
	}
	if client == nil {
		return nil, fmt.Errorf("%w: client is empty", ErrInvalidLocker)
	}
	config := NewConfigBuilder().
		Labels(nil).
		CleanupLease(false).
		Build()
	for _, f := range opt {
		f(config)
	}
	return &Locker{
		namespace:   namespace,
		name:        name,
		id:          id,
		client:      client,
		labels:      config.Labels.Get(),
		needCleanup: config.CleanupLease.Get(),
	}, nil
}

// Locker runs the given function under lock control.
type Locker struct {
	namespace   string
	name        string
	id          string
	client      coordinationv1client.LeasesGetter
	labels      labels.Set
	needCleanup bool
}

func (s *Locker) Namespace() string { return s.namespace }
func (s *Locker) Name() string      { return s.name }
func (s *Locker) ID() string        { return s.id }

func (s *Locker) String() string {
	return fmt.Sprintf("namespace=%s name=%s id=%s", s.namespace, s.name, s.id)
}

func (s *Locker) Logger() *slog.Logger {
	return slog.With(
		slog.String("namespace", s.namespace),
		slog.String("name", s.name),
		slog.String("id", s.id),
	)
}

func (s *Locker) Labels() labels.Set {
	if len(s.labels) == 0 {
		return CommonLabels()
	}
	return labels.Merge(s.labels, CommonLabels())
}

// LockAndRun tries to call f with the lease.
//
// Do the following:
//
//   - try to acquire leadership
//   - abort if the leader election timed out
//   - invoke `f` when leadership is acquired
//   - perform cleanup if `f` was invoked and the process is aborted or cancelled and leadership is lost
//   - delete the lease if needed
//
// Available options:
//
//   - WithCleanup: the function for the cleanup
//   - WithLeaderElectTimeout: the timeout of the leader election (default: unlimited(0))
func (s *Locker) LockAndRun(ctx context.Context, f func(context.Context), opt ...ConfigOption) error {
	config := NewConfigBuilder().
		Cleanup(nil).
		LeaderElectTimeout(0).
		Build()
	for _, f := range opt {
		f(config)
	}

	ctx, cancel := context.WithCancel(ctx)
	logger := s.Logger()

	//
	// For LeaderElectTimeout
	//
	startedC := make(chan struct{})
	type electResultType int
	const (
		electSucceeded electResultType = iota
		electTimedOut
		electCanceled
	)
	electResultC := make(chan electResultType)
	go func() {
		defer close(electResultC)
		timeout := config.LeaderElectTimeout.Get()
		logger.Debug("Waiting the leader election", slog.String("timeout", timeout.String()))
		if timeout == 0 {
			timeout = time.Hour * 24 * 365 * 100 // 100 years
		}
		select {
		case <-ctx.Done():
			electResultC <- electCanceled
		case <-time.After(timeout):
			logger.Info("Aborting the process because the leader election timed out")
			cancel()
			electResultC <- electTimedOut
		case <-startedC:
			logger.Info("Starting the process because the leader election succeeded")
			electResultC <- electSucceeded
		}
	}()

	var (
		leaseLock = &resourcelock.LeaseLock{
			LeaseMeta: metav1.ObjectMeta{
				Namespace: s.namespace,
				Name:      s.name,
			},
			Client: s.client,
			LockConfig: resourcelock.ResourceLockConfig{
				Identity: s.id,
			},
			Labels: s.Labels(),
		}

		started   atomic.Bool
		callbacks = leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				started.Store(true)
				close(startedC) // notify started leading
				logger.Debug("Become leader")
				if f != nil {
					f(ctx)
					cancel()
				}
			},
			OnStoppedLeading: func() {
				logger.Debug("Lost leader")
				if started.Load() {
					if g := config.Cleanup.Get(); g != nil {
						logger.Debug("Cleanup")
						g()
					}
				}
			},
			OnNewLeader: func(identity string) {
				if s.id == identity {
					return
				}
				logger.Debug("Leader elected", slog.String("id", identity))
			},
		}
		electionConfig = leaderelection.LeaderElectionConfig{
			Lock:            leaseLock,
			ReleaseOnCancel: true,
			LeaseDuration:   15 * time.Second, // Core clients default
			RenewDeadline:   10 * time.Second, // Core clients default
			RetryPeriod:     2 * time.Second,  // Core clients default
			Callbacks:       callbacks,
		}
	)

	leaderelection.RunOrDie(ctx, electionConfig)
	var errs []error
	switch <-electResultC {
	case electTimedOut:
		errs = append(errs, ErrElectTimedOut)
	case electCanceled:
		errs = append(errs, ctx.Err())
	}
	cancel()

	if s.needCleanup {
		logger.Debug("Cleanup lease")
		if err := s.cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("%w: failed to cleanup lease: %s", err, s))
		}
	}
	return errors.Join(errs...)
}

// Delete the created lease.
func (s *Locker) cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := s.client.Leases(s.namespace)
	x, err := c.Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return c.Delete(ctx, s.name, metav1.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationBackground),
		Preconditions: &metav1.Preconditions{
			UID: ptr.To(x.GetUID()),
		},
	})
}
