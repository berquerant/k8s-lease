package logging

import (
	"context"

	"k8s.io/klog/v2"
)

func FromContext(ctx context.Context) klog.Logger {
	return klog.FromContext(ctx).WithValues("component", "k8s-lease-klock")
}
