/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package migration handles one-time cluster-state migrations.
package migration

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	oldGroup = "metrics.openmcp.cloud"
	newGroup = "metrics.open-control-plane.io"
	version  = "v1alpha1"
)

// kinds lists every resource kind (lowercase plural) in the API group.
var kinds = []schema.GroupVersionResource{
	{Group: oldGroup, Version: version, Resource: "datasinks"},
	{Group: oldGroup, Version: version, Resource: "federatedclusteraccesses"},
	{Group: oldGroup, Version: version, Resource: "federatedmanagedmetrics"},
	{Group: oldGroup, Version: version, Resource: "federatedmetrics"},
	{Group: oldGroup, Version: version, Resource: "managedmetrics"},
	{Group: oldGroup, Version: version, Resource: "metrics"},
	{Group: oldGroup, Version: version, Resource: "remoteclusteraccesses"},
}

// MigrateAPIGroup copies all resources from the old API group to the new one
// and removes the old CRDs. It is idempotent: if no old CRDs are present it
// returns immediately.
func MigrateAPIGroup(ctx context.Context, c client.Client) error {
	log := log.FromContext(ctx).WithName("migration")

	// Fast-path: check whether any old CRD still exists.
	oldCRDs, err := listOldCRDs(ctx, c)
	if err != nil {
		return fmt.Errorf("checking for old CRDs: %w", err)
	}
	if len(oldCRDs) == 0 {
		log.V(1).Info("no old CRDs found, skipping migration")
		return nil
	}

	log.Info("old API group CRDs found, starting migration", "count", len(oldCRDs), "from", oldGroup, "to", newGroup)

	for _, gvr := range kinds {
		if err := migrateKind(ctx, c, gvr); err != nil {
			return fmt.Errorf("migrating %s: %w", gvr.Resource, err)
		}
	}

	// Remove old CRDs now that all resources have been migrated.
	for i := range oldCRDs {
		crd := &oldCRDs[i]
		log.Info("deleting old CRD", "name", crd.Name)
		if err := c.Delete(ctx, crd); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting CRD %s: %w", crd.Name, err)
		}
	}

	log.Info("migration complete")
	return nil
}

// listOldCRDs returns all CRDs whose group is the old group.
func listOldCRDs(ctx context.Context, c client.Client) ([]apiextensionsv1.CustomResourceDefinition, error) {
	list := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := c.List(ctx, list); err != nil {
		return nil, err
	}
	var old []apiextensionsv1.CustomResourceDefinition
	for _, crd := range list.Items {
		if crd.Spec.Group == oldGroup {
			old = append(old, crd)
		}
	}
	return old, nil
}

// migrateKind migrates all namespaced resources of a single kind.
func migrateKind(ctx context.Context, c client.Client, gvr schema.GroupVersionResource) error {
	log := log.FromContext(ctx).WithName("migration").WithValues("resource", gvr.Resource)

	oldList := &unstructured.UnstructuredList{}
	oldList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    resourceToListKind(gvr.Resource),
	})

	if err := c.List(ctx, oldList); err != nil {
		if errors.IsNotFound(err) || isNoKindMatch(err) {
			log.V(1).Info("no resources found (CRD may already be gone)")
			return nil
		}
		return err
	}

	for i := range oldList.Items {
		old := &oldList.Items[i]
		if err := migrateObject(ctx, c, old); err != nil {
			return fmt.Errorf("%s/%s: %w", old.GetNamespace(), old.GetName(), err)
		}
	}
	return nil
}

func migrateObject(ctx context.Context, c client.Client, old *unstructured.Unstructured) error {
	log := log.FromContext(ctx).WithName("migration").WithValues(
		"resource", old.GetKind(),
		"namespace", old.GetNamespace(),
		"name", old.GetName(),
	)

	// Build the new object: rewrite apiVersion, strip server-managed fields.
	obj := old.DeepCopy()
	obj.SetAPIVersion(newGroup + "/" + version)
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetGeneration(0)
	obj.SetManagedFields(nil)
	// Clear status — the controller will repopulate it.
	delete(obj.Object, "status")

	log.Info("applying to new group")
	if err := c.Create(ctx, obj); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create in new group: %w", err)
	}

	log.Info("deleting from old group")
	if err := c.Delete(ctx, old); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete from old group: %w", err)
	}

	return nil
}

// resourceToListKind converts a lowercase plural resource name to its List GVK.
// e.g. "metrics" → "MetricList", "datasinks" → "DataSinkList"
func resourceToListKind(resource string) string {
	return resourceToKind(resource) + "List"
}

// resourceToKind maps plural resource names to their singular Kind.
var resourceKindMap = map[string]string{
	"datasinks":                "DataSink",
	"federatedclusteraccesses": "FederatedClusterAccess",
	"federatedmanagedmetrics":  "FederatedManagedMetric",
	"federatedmetrics":         "FederatedMetric",
	"managedmetrics":           "ManagedMetric",
	"metrics":                  "Metric",
	"remoteclusteraccesses":    "RemoteClusterAccess",
}

func resourceToKind(resource string) string {
	if k, ok := resourceKindMap[resource]; ok {
		return k
	}
	return resource
}

// isNoKindMatch returns true when the API server doesn't know the GVK at all
// (i.e. the CRD was already deleted before we tried to list).
func isNoKindMatch(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*errors.StatusError)
	if ok {
		return false
	}
	// controller-runtime wraps no-match errors as plain errors
	return containsAny(err.Error(), "no kind is registered", "no matches for kind", "no resource type")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
