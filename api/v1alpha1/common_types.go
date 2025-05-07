package v1alpha1

import "k8s.io/apimachinery/pkg/runtime/schema"

// GroupVersionKind defines the group, version and kind of the object that should be instrumented
type GroupVersionKind struct {
	// Define the kind of the object that should be instrumented
	Kind string `json:"kind,omitempty"`
	// Define the group of your object that should be instrumented
	Group string `json:"group,omitempty"`
	// Define version of the object you want to be instrumented
	Version string `json:"version,omitempty"`
}

func (gvk *GroupVersionKind) GVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Kind:    gvk.Kind,
		Version: gvk.Version,
	}
}
