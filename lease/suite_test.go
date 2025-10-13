package lease_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/berquerant/k8s-lease/kconfig"
	"github.com/berquerant/k8s-lease/lease"
	"github.com/berquerant/k8s-lease/logging"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestLocker(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(2 * time.Second)
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultConsistentlyDuration(2 * time.Second)
	SetDefaultConsistentlyPollingInterval(100 * time.Millisecond)
	RunSpecs(t, "Locker Suite")
}

var (
	ctx         context.Context
	cancel      context.CancelFunc
	clientSet   *clientset.Clientset
	clientIface coordinationv1client.CoordinationV1Interface
	client      coordinationv1client.LeaseInterface
)

func listLeases(ctx context.Context) (*coordinationv1.LeaseList, error) {
	return client.List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(lease.CommonLabels()).String(),
	})
}

func getLease(ctx context.Context, name string) (*coordinationv1.Lease, error) {
	return client.Get(ctx, name, metav1.GetOptions{})
}

func cleanupLeases(ctx context.Context) error {
	return client.DeleteCollection(ctx, metav1.DeleteOptions{
		PropagationPolicy: ptr.To(metav1.DeletePropagationForeground),
	}, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(lease.CommonLabels()).String(),
	})
}

func ensureCleanupLeases(ctx context.Context) {
	Eventually(func() error {
		if err := cleanupLeases(ctx); err != nil {
			return err
		}
		_, err := listLeases(ctx)
		return err
	}).Should(Succeed())
}

const (
	namespace = "default"
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.TODO())
	By("setup loggers")
	logging.Setup(os.Stdout, os.Getenv("DEBUG") != "")
	logf.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))
	By("setup clients")
	cfg, err := kconfig.Build("")
	Expect(err).To(Succeed())
	Expect(cfg).NotTo(BeNil())
	// relax ratelimit due to testing
	cfg.QPS = 100
	cfg.Burst = 100
	clientSet = clientset.NewForConfigOrDie(cfg)
	Expect(clientSet).NotTo(BeNil())
	clientIface = clientSet.CoordinationV1()
	Expect(clientIface).NotTo(BeNil())
	client = clientIface.Leases(namespace)
	Expect(client).NotTo(BeNil())
	ensureCleanupLeases(ctx)
})

var _ = AfterSuite(func() {
	By("teardown")
	cancel()
	ensureCleanupLeases(context.TODO())
})
