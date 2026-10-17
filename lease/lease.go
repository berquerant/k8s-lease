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

const (
	// DefaultLeaseDuration is the total time a leader holds the lock before it expires.
	DefaultLeaseDuration = 15 * time.Second
	// DefaultRenewDeadline is the time limit for the leader to renew its lock.
	DefaultRenewDeadline = 10 * time.Second
	// DefaultRetryPeriod is the interval between lock acquisition/renewal attempts.
	DefaultRetryPeriod = 2 * time.Second

	// cleanupTimeout is the timeout for deleting the Lease resource after processing.
	cleanupTimeout = 5 * time.Second
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
//   - WithRenewDeadline: the time limit for the leader to successfully renew its lock before stepping down (default: 10 seconds)
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
		LeaseDuration(DefaultLeaseDuration).
		RenewDeadline(DefaultRenewDeadline).
		RetryPeriod(DefaultRetryPeriod).
		LeaderElectTimeout(0).
		Build()
	for _, f := range opt {
		f(config)
	}
	var (
		leaseDuration      = config.LeaseDuration.Get()
		renewDeadline      = config.RenewDeadline.Get()
		retryPeriod        = config.RetryPeriod.Get()
		leaderElectTimeout = config.LeaderElectTimeout.Get()
	)
	if err := validateDurations(leaseDuration, renewDeadline, retryPeriod, leaderElectTimeout); err != nil {
		return nil, err
	}
	return &Locker{
		namespace:          namespace,
		name:               name,
		id:                 id,
		client:             client,
		labels:             config.Labels.Get(),
		needCleanup:        config.CleanupLease.Get(),
		leaseDuration:      leaseDuration,
		renewDeadline:      renewDeadline,
		retryPeriod:        retryPeriod,
		leaderElectTimeout: leaderElectTimeout,
	}, nil
}

func validateDurations(leaseDuration, renewDeadline, retryPeriod, leaderElectTimeout time.Duration) error {
	if leaseDuration < 1 {
		return fmt.Errorf("%w: leaseDuration must be greater than zero", ErrInvalidLocker)
	}
	if renewDeadline < 1 {
		return fmt.Errorf("%w: renewDeadline must be greater than zero", ErrInvalidLocker)
	}
	if retryPeriod < 1 {
		return fmt.Errorf("%w: retryPeriod must be greater than zero", ErrInvalidLocker)
	}
	if leaseDuration <= renewDeadline {
		return fmt.Errorf("%w: leaseDuration must be greater than renewDeadline", ErrInvalidLocker)
	}
	if renewDeadline <= time.Duration(leaderelection.JitterFactor*float64(retryPeriod)) {
		return fmt.Errorf("%w: renewDeadline must be greater than retryPeriod*JitterFactor", ErrInvalidLocker)
	}
	if leaderElectTimeout < 0 {
		return fmt.Errorf("%w: leaderElectTimeout must be non-negative", ErrInvalidLocker)
	}
	return nil
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

	parentCtx := ctx // keep a reference before WithCancel for use in cleanup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		logger                = s.Logger(ctx)
		startedC              = make(chan struct{})
		onStartedLeadingDoneC = make(chan error, 1)
		electionConfig        = s.newElectionConfig(ctx, cancel, startedC, onStartedLeadingDoneC, f)
	)
	le, err := leaderelection.NewLeaderElector(electionConfig)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidLocker, err)
	}

	electResultC := s.watchElection(ctx, cancel, startedC)
	le.Run(ctx)
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
		if err := s.cleanup(parentCtx); err != nil {
			errs = append(errs, fmt.Errorf("%w: failed to cleanup lease: %s", err, s))
		}
	}
	return errors.Join(errs...)
}

type electResultType int

const (
	electSucceeded electResultType = iota
	electTimedOut
	electCanceled
)

func (s *Locker) watchElection(ctx context.Context, cancel context.CancelFunc, startedC <-chan struct{}) <-chan electResultType {
	logger := s.Logger(ctx)
	electResultC := make(chan electResultType, 1)
	go func() {
		defer close(electResultC)
		logger.V(1).Info("waiting the leader election", "timeout", s.leaderElectTimeout)
		var timeoutC <-chan time.Time
		if s.leaderElectTimeout > 0 {
			timeoutC = time.After(s.leaderElectTimeout)
		}
		select {
		case <-ctx.Done():
			electResultC <- electCanceled
		case <-timeoutC:
			logger.V(0).Info("aborting the process because the leader election timed out")
			cancel()
			electResultC <- electTimedOut
		case <-startedC:
			logger.V(0).Info("starting the process because the leader election succeeded")
			electResultC <- electSucceeded
		}
	}()
	return electResultC
}

func (s *Locker) newElectionConfig(
	ctx context.Context,
	cancel context.CancelFunc,
	startedC chan<- struct{},
	onDoneC chan<- error,
	f func(context.Context) error,
) leaderelection.LeaderElectionConfig {
	return leaderelection.LeaderElectionConfig{
		Lock: &resourcelock.LeaseLock{
			LeaseMeta: metav1.ObjectMeta{
				Namespace: s.namespace,
				Name:      s.name,
			},
			Client: s.client,
			LockConfig: resourcelock.ResourceLockConfig{
				Identity: s.id,
			},
			Labels: s.Labels(),
		},
		ReleaseOnCancel: true,
		LeaseDuration:   s.leaseDuration,
		RenewDeadline:   s.renewDeadline,
		RetryPeriod:     s.retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				close(startedC)
				s.Logger(ctx).V(1).Info("become leader")
				err := f(ctx)
				cancel()
				onDoneC <- err
			},
			OnStoppedLeading: func() {
				s.Logger(ctx).V(1).Info("lost leader")
				cancel()
			},
			OnNewLeader: func(identity string) {
				if s.id != identity {
					s.Logger(ctx).V(1).Info("leader elected", "id", identity)
				}
			},
		},
	}
}

// cleanup deletes the created lease.
// parentCtx is the context passed to LockAndRun before internal cancellation;
// it remains valid for external signals (e.g. SIGTERM) during cleanup.
func (s *Locker) cleanup(parentCtx context.Context) error {
	ctx, cancel := context.WithTimeout(parentCtx, cleanupTimeout)
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
