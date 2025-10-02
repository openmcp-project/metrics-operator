package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

func TestGetManagedResources(t *testing.T) {
	// we define a couple of GVKs to generate CRDs and resources for our test cases
	k8sObjectGVK := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}
	k8sObjectCollectionGVK := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "ObservedObjectCollection",
	}
	nopResourceGVK := schema.GroupVersionKind{
		Group:   "nop.crossplane.io",
		Version: "v1alpha1",
		Kind:    "NopResource",
	}
	helmReleaseGVK := schema.GroupVersionKind{
		Group:   "helm.m.crossplane.io",
		Version: "v1beta1",
		Kind:    "Release",
	}

	const (
		k8sObjects           = "k8s-object"
		k8sObjectCollections = "k8s-object-collection"
		nopResources         = "nop"
		helmReleases         = "helm"
	)

	// and a couple of fixed cluster resources
	resourceFixture := map[string][]string{
		k8sObjects: {
			fakeResource(k8sObjectGVK),
			fakeResource(k8sObjectGVK),
		},
		k8sObjectCollections: {
			fakeResource(k8sObjectCollectionGVK),
			fakeResource(k8sObjectCollectionGVK),
		},
		nopResources: {
			fakeResource(nopResourceGVK),
			fakeResource(nopResourceGVK),
		},
		helmReleases: {
			fakeResource(helmReleaseGVK),
			fakeResource(helmReleaseGVK),
		},
	}

	tests := []struct {
		name             string
		filter           schema.GroupVersionKind
		clusterCRDs      []string
		clusterResources []string
		wantResources    []string
	}{
		{
			name:   "fully qualified target spec",
			filter: k8sObjectGVK,
			clusterCRDs: []string{
				managedAndServedCRD(k8sObjectGVK),
				managedAndServedCRD(k8sObjectCollectionGVK),
				managedAndServedCRD(nopResourceGVK),
				managedAndServedCRD(helmReleaseGVK),
			},
			clusterResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
			wantResources: resourceFixture[k8sObjects],
		},
		{
			name: "group version target",
			filter: schema.GroupVersionKind{
				Group:   k8sObjectGVK.Group,
				Version: k8sObjectGVK.Version,
			},
			clusterCRDs: []string{
				managedAndServedCRD(k8sObjectGVK),
				managedAndServedCRD(k8sObjectCollectionGVK),
				managedAndServedCRD(nopResourceGVK),
				managedAndServedCRD(helmReleaseGVK),
			},
			clusterResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
			wantResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
			),
		},
		{
			name: "version target",
			filter: schema.GroupVersionKind{
				Version: k8sObjectGVK.Version,
			},
			clusterCRDs: []string{
				managedAndServedCRD(k8sObjectGVK),
				managedAndServedCRD(k8sObjectCollectionGVK),
				managedAndServedCRD(nopResourceGVK),
				managedAndServedCRD(helmReleaseGVK),
			},
			clusterResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
			wantResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
			),
		},
		{
			name:   "unqualified target",
			filter: schema.GroupVersionKind{},
			clusterCRDs: []string{
				managedAndServedCRD(k8sObjectGVK),
				managedAndServedCRD(k8sObjectCollectionGVK),
				managedAndServedCRD(nopResourceGVK),
				managedAndServedCRD(helmReleaseGVK),
			},
			clusterResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
			wantResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
		},
		{
			name:   "unmanaged custom resources get filtered out",
			filter: schema.GroupVersionKind{},
			clusterCRDs: []string{
				unmanagedCRD(k8sObjectGVK),
				managedAndServedCRD(k8sObjectCollectionGVK),
				unmanagedCRD(nopResourceGVK),
				managedAndServedCRD(helmReleaseGVK),
			},
			clusterResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
			wantResources: slices.Concat(
				resourceFixture[k8sObjectCollections],
				resourceFixture[helmReleases],
			),
		},
		{
			name:   "unserved custom resources are not retrievable",
			filter: schema.GroupVersionKind{},
			clusterCRDs: []string{
				unservedCRD(k8sObjectGVK),
				managedAndServedCRD(k8sObjectCollectionGVK),
				managedAndServedCRD(nopResourceGVK),
				unservedCRD(helmReleaseGVK),
			},
			clusterResources: slices.Concat(
				resourceFixture[k8sObjects],
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
				resourceFixture[helmReleases],
			),
			wantResources: slices.Concat(
				resourceFixture[k8sObjectCollections],
				resourceFixture[nopResources],
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup handler
			handler := ManagedHandler{
				client: setupFakeClient(t, tt.clusterCRDs),
				dCli:   setupFakeDynamicClient(t, tt.clusterResources),
				metric: v1alpha1.ManagedMetric{
					Spec: v1alpha1.ManagedMetricSpec{
						Kind:    tt.filter.Kind,
						Group:   tt.filter.Group,
						Version: tt.filter.Version,
					},
				},
			}

			// execute getManagedResources
			result, err := handler.getManagedResources(context.Background())
			if err != nil {
				t.Fatalf("getManagedResource failed: %v", err)
			}

			// verify result
			if len(tt.wantResources) != len(result) {
				t.Errorf("unexpected result length: wanted=%v, got=%v", len(tt.wantResources), len(result))
			}
			for _, managed := range result {
				if !slices.ContainsFunc(tt.wantResources, func(yaml string) bool {
					left := yamlNameGVK(t, yaml)
					right := managedNameGVK(t, managed)
					return left == right
				}) {
					t.Errorf("unexpected resource: %v", managedNameGVK(t, managed))
				}
			}
		})
	}
}

func setupFakeClient(t *testing.T, yamlCRDs []string) client.WithWatch {
	t.Helper()

	// general runtime setup
	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)

	// setup fake crd result
	result := make([]client.Object, 0, len(yamlCRDs))
	for _, yamlItem := range yamlCRDs {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := yaml.Unmarshal([]byte(yamlItem), &crd); err != nil {
			t.Fatalf("failed to unmarshal test CRD: %v", err)
		}
		result = append(result, &crd)
	}

	// setup fake client
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(result...).
		Build()
}

func setupFakeDynamicClient(t *testing.T, yamlResources []string) *dynamicfake.FakeDynamicClient {
	t.Helper()

	// general runtime setup
	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)

	// setup fake managed resources result
	fakeObjects := make([]runtime.Object, 0, len(yamlResources))
	for _, yamlItem := range yamlResources {
		obj := toUnstructured(t, yamlItem)
		fakeObjects = append(fakeObjects, &obj)
	}

	// setup fake dynamic client
	return dynamicfake.NewSimpleDynamicClient(scheme, fakeObjects...)
}

func managedNameGVK(t *testing.T, managed Managed) string {
	t.Helper()
	gv, err := schema.ParseGroupVersion(managed.APIVersion)
	if err != nil {
		t.Errorf("failed to parse managed group version: %v", err)
	}
	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    managed.Kind,
	}
	return fmt.Sprintf("%v:%v", gvk, managed.Metadata.Name)
}

func yamlNameGVK(t *testing.T, yaml string) string {
	t.Helper()
	obj := toUnstructured(t, yaml)
	return fmt.Sprintf("%v:%v", obj.GetObjectKind().GroupVersionKind(), obj.GetName())
}

func fakeResource(gvk schema.GroupVersionKind) string {
	return fmt.Sprintf(`apiVersion: %v
kind: %v 
metadata:
  name: %v
spec:
  deletionPolicy: Delete
status:
  conditions:
  - lastTransitionTime: "2025-09-12T15:57:41Z"
    observedGeneration: 1
    reason: ReconcileSuccess
    status: "True"
    type: Synced
  - lastTransitionTime: "2025-09-09T14:33:38Z"
    reason: Available
    status: "True"
    type: Ready
`,
		gvk.GroupVersion(),
		gvk.Kind,
		rand.String(16))
}

func managedAndServedCRD(gvk schema.GroupVersionKind) string {
	return fakeCRDTemplate(gvk, true, true)
}

func unservedCRD(gvk schema.GroupVersionKind) string {
	return fakeCRDTemplate(gvk, true, false)
}

func unmanagedCRD(gvk schema.GroupVersionKind) string {
	return fakeCRDTemplate(gvk, false, true)
}

func fakeCRDTemplate(gvk schema.GroupVersionKind, managed bool, served bool) string {
	categories := "[]"
	if managed {
		categories = `
    - crossplane
    - managed`
	}
	return fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %vs.%v
spec:
  group: %v
  names:
    categories: %v
    kind: %v
    listKind: %vList
    plural: %vs
    singular: %v
  scope: Cluster
  versions:
  - name: %v
    served: %v
`,
		strings.ToLower(gvk.Kind),
		gvk.Group,
		gvk.Group,
		categories,
		gvk.Kind,
		gvk.Kind,
		strings.ToLower(gvk.Kind),
		strings.ToLower(gvk.Kind),
		gvk.Version,
		served)
}
