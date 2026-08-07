package lease

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/berquerant/k8s-lease/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

var (
	ErrInvalidLocker = errors.New("InvalidLocker")
	ErrElectTimedOut = errors.New("ElectTimedOut")
)

//go:generate go tool goconfig -field "Labels labels.Set|CleanupLease bool|LeaderElectTimeout time.Duration|LeaseDuration time.Duration|RenewDeadline time.Duration|RetryPeriod time.Duration" -option -output config_generated.go

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
//   - WithLeaseDuration: the total time a leader node holds the lock before it expires (default: 15 seconds)
//   - WithRenewDuration: the time limit for the leader to successfully renew its lock before stepping down (default: 10 seconds)
//   - WithRetryPeriod: the time interval between each attempt to acquire or renew the lock (default: 2 seconds)
//   - WithLeaderElectTimeout: the timeout of the leader election (default: unlimited(0))
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
		return nil, fmt.Errorf("%w: client is nil", ErrInvalidLocker)
	}
	config := NewConfigBuilder().
		Labels(nil).
		CleanupLease(false).
		LeaseDuration(15 * time.Second).
		RenewDeadline(10 * time.Second).
		RetryPeriod(2 * time.Second).
		LeaderElectTimeout(0).
		Build()
	for _, f := range opt {
		f(config)
	}
	return &Locker{
		namespace:          namespace,
		name:               name,
		id:                 id,
		client:             client,
		labels:             config.Labels.Get(),
		needCleanup:        config.CleanupLease.Get(),
		leaseDuration:      config.LeaseDuration.Get(),
		renewDeadline:      config.RenewDeadline.Get(),
		retryPeriod:        config.RetryPeriod.Get(),
		leaderElectTimeout: config.LeaderElectTimeout.Get(),
	}, nil
}

// Locker runs the given function under lock control.
type Locker struct {
	namespace                                                     string
	name                                                          string
	id                                                            string
	client                                                        coordinationv1client.LeasesGetter
	labels                                                        labels.Set
	needCleanup                                                   bool
	leaderElectTimeout, leaseDuration, renewDeadline, retryPeriod time.Duration
}

func (s *Locker) Namespace() string { return s.namespace }
func (s *Locker) Name() string      { return s.name }
func (s *Locker) ID() string        { return s.id }

func (s *Locker) String() string {
	return fmt.Sprintf("namespace=%s name=%s id=%s", s.namespace, s.name, s.id)
}

func (s *Locker) Logger(ctx context.Context) klog.Logger {
	return logging.FromContext(ctx).WithValues(
		"namespace", s.namespace,
		"name", s.name,
		"id", s.id,
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
//   - delete the lease if needed
func (s *Locker) LockAndRun(ctx context.Context, f func(context.Context) error) error {
	if f == nil {
		return fmt.Errorf("%w: f is nil", ErrInvalidLocker)
	}

	ctx, cancel := context.WithCancel(ctx)
	logger := s.Logger(ctx)

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
		logger.V(1).Info("waiting the leader election", "timeout", s.leaderElectTimeout)
		if s.leaderElectTimeout == 0 {
			// No timeout: wait indefinitely for ctx cancellation or leadership.
			select {
			case <-ctx.Done():
				electResultC <- electCanceled
			case <-startedC:
				logger.V(0).Info("starting the process because the leader election succeeded")
				electResultC <- electSucceeded
			}
		} else {
			select {
			case <-ctx.Done():
				electResultC <- electCanceled
			case <-time.After(s.leaderElectTimeout):
				logger.V(0).Info("aborting the process because the leader election timed out")
				cancel()
				electResultC <- electTimedOut
			case <-startedC:
				logger.V(0).Info("starting the process because the leader election succeeded")
				electResultC <- electSucceeded
			}
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

		onStartedLeadingDoneC = make(chan error)
		callbacks             = leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				close(startedC) // notify started leading
				logger.V(1).Info("become leader")
				err := f(ctx)
				cancel()
				onStartedLeadingDoneC <- err
			},
			OnStoppedLeading: func() {
				logger.V(1).Info("lost leader")
				cancel()
			},
			OnNewLeader: func(identity string) {
				if s.id == identity {
					return
				}
				logger.V(1).Info("leader elected", "id", identity)
			},
		}
		electionConfig = leaderelection.LeaderElectionConfig{
			Lock:            leaseLock,
			ReleaseOnCancel: true,
			LeaseDuration:   s.leaseDuration,
			RenewDeadline:   s.renewDeadline,
			RetryPeriod:     s.retryPeriod,
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
	case electSucceeded:
		errs = append(errs, <-onStartedLeadingDoneC)
	}
	cancel()

	if s.needCleanup {
		logger.V(1).Info("cleanup lease")
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
			UID: new(x.GetUID()),
		},
	})
}
