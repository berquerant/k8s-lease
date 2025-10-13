package lease

import "k8s.io/apimachinery/pkg/labels"

const (
	toolName  = "k8s-lease-klock"
	managedBy = "app.kubernetes.io/managed-by"
)

// CommonLabels returns the common labels for the leases created by k8s-lease.
func CommonLabels() labels.Set {
	return map[string]string{
		managedBy: toolName,
	}
}

func ParseLabelsFromString(selector string) (labels.Set, error) {
	return labels.ConvertSelectorToLabelsMap(selector)
}

func LabelsIntoString(labs labels.Set) string {
	return labels.FormatLabels(labs)
}
