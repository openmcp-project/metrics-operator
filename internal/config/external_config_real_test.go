package config

//
// import (
//	"context"
//	"fmt"
//	"testing"
//
//	v1 "github.com/SAP/metrics-operator/api/v1alpha1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/runtime/schema"
//	"k8s.io/client-go/dynamic"
//	"k8s.io/client-go/tools/clientcmd"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//)
//
// func TestCreateExternalQueryConfig_REAL(t *testing.T) {
//
//	ctx := context.TODO()
//
//	rcaRef := &v1.RemoteClusterAccessRef{Name: "crate-cluster", Namespace: "test-monitoring"}
//
//	restconfig, errrc := clientcmd.BuildConfigFromFlags("", "/Users/I073426/Desktop/metric-demo/dev-core.yaml")
//	if errrc != nil {
//		fmt.Println(errrc)
//	}
//
//	// Test parameters
//	//saName := "mcp-operator"
//	//saNamespace := "cola-system"
//	//audience := "crate"
//
//	// Create the client
//	inClient, err := client.New(restconfig, client.Options{Scheme: externalScheme})
//
//	queryConfig, err := CreateExternalQueryConfig(ctx, rcaRef, inClient)
//	if err != nil {
//		t.Errorf("Error: %v", err)
//	}
//
//	println(queryConfig)
//
//	dynamicClient, errDyn := dynamic.NewForConfig(&queryConfig.RestConfig)
//	if errDyn != nil {
//		fmt.Println(errDyn)
//	}
//
//	gvr := schema.GroupVersionResource{
//		Group:    "cola.cloud.sap",
//		Version:  "v1alpha1",
//		Resource: "managedcontrolplanes",
//	}
//
//	unstrct, err := dynamicClient.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
//	if err != nil {
//
//		//Project.cola.cloud.sap "valentin" is forbidden: User "dev-core:system:serviceaccount:cola-system:mcp-operator" cannot get resource "Project" in API group "cola.cloud.sap" in the namespace "project-valentin"
//
//		// Handle error
//		fmt.Println(err)
//	}
//
//	println(unstrct)
//}
