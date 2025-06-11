package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/orchestrator"
)

const (
	caDataKey                  = "caData"
	audienceKey                = "audience"
	hostKey                    = "host"
	defaultKubeconfigSecretKey = "kubeconfig"
)

var (
	externalScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(externalScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(externalScheme))
	utilruntime.Must(v1alpha1.AddToScheme(externalScheme))

}

// CreateExternalQueryConfig creates an external query config from a remote cluster access reference
func CreateExternalQueryConfig(ctx context.Context, racRef *v1alpha1.RemoteClusterAccessRef, inClient client.Client) (*orchestrator.QueryConfig, error) {

	rcaName := racRef.Name
	rcaNamespace := racRef.Namespace

	rca := &v1alpha1.RemoteClusterAccess{}
	err := inClient.Get(ctx, types.NamespacedName{Name: rcaName, Namespace: rcaNamespace}, rca)
	if err != nil {
		errRCA := fmt.Errorf("failed to retrieve Remote Cluster Acces Ref with name %s in namespace %s: %w", rcaName, rcaNamespace, err)
		return nil, errRCA
	}

	kcRef := rca.Spec.KubeConfigSecretRef
	if kcRef != nil {
		return queryConfigFromKubeConfig(ctx, kcRef, inClient, externalScheme)
	}

	cac := rca.Spec.ClusterAccessConfig
	if cac != nil {
		return queryConfigFromClusterAccessConfig(ctx, cac, inClient, externalScheme)
	}

	return nil, fmt.Errorf("kubeconfigSecretRef and clusterAccessConfig are both nil")
}

func queryConfigFromClusterAccessConfig(ctx context.Context, cac *v1alpha1.ClusterAccessConfig, inClient client.Client, externalScheme *runtime.Scheme) (*orchestrator.QueryConfig, error) {
	clsData, errData := getCusterDataFromSecret(ctx, cac, inClient)
	if errData != nil {
		return nil, errData
	}

	saName := cac.ServiceAccountName
	saNamespace := cac.ServiceAccountNamespace

	token, errToken := getTokenWithAPI(ctx, inClient, saName, saNamespace, clsData.audience)
	if errToken != nil {
		return nil, errToken
	}

	// Create a restconfig from token, host, caData, and audience

	restConfig := &rest.Config{
		Host:        clsData.host,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(clsData.caData),
		},
	}

	// Create the client
	externalClient, err := client.New(restConfig, client.Options{Scheme: externalScheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create external client: %w", err)

	}

	parsedHost, errParse := url.Parse(clsData.host)
	if errParse != nil {
		return nil, fmt.Errorf("failed to parse host URL: %w", errParse)
	}
	hostName := parsedHost.Hostname()

	return &orchestrator.QueryConfig{Client: externalClient, RestConfig: *restConfig, ClusterName: &hostName}, nil
}

func queryConfigFromKubeConfig(ctx context.Context, kcRef *v1alpha1.KubeConfigSecretRef, inClient client.Client, externalScheme *runtime.Scheme) (*orchestrator.QueryConfig, error) {
	secretName := kcRef.Name
	secretNamespace := kcRef.Namespace

	// Retrieve the Secret
	secret := &corev1.Secret{}
	err := inClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		errSecret := fmt.Errorf("failed to retrieve KubeConfig Secret Ref with name %s in namespace %s: %w", secretName, secretNamespace, err)
		return nil, errSecret
	}

	key := kcRef.Key
	kubeconfigData, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("kubeconfig key %s not found in Secret", key)
	}

	// Create a config from the kubeconfig data
	config, errRest := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if errRest != nil {
		return nil, fmt.Errorf("failed to create config from kubeconfig: %w", err)
	}

	kubeconfig, errKC := clientcmd.Load(kubeconfigData)
	if errKC != nil {
		return nil, fmt.Errorf("failed to load Config object from kubeconfigData: %w", errKC)
	}

	clusterName := kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster

	// Create the client
	externalClient, err := client.New(config, client.Options{Scheme: externalScheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &orchestrator.QueryConfig{Client: externalClient, RestConfig: *config, ClusterName: &clusterName}, nil
}

func getTokenWithAPI(ctx context.Context, inClient client.Client, serviceAccount, namespace, audience string) (string, error) {
	tm, errTM := GetTokenManager(inClient)

	if errTM != nil {
		return "", fmt.Errorf("failed to get token manager: %w", errTM)
	}

	token, errTK := tm.GetToken(ctx, namespace, serviceAccount, audience)

	if errTK != nil {
		return "", fmt.Errorf("failed to get token for %s/%s/%s: %w", namespace, serviceAccount, audience, errTK)
	}

	return token, nil
}

func getCusterDataFromSecret(ctx context.Context, cac *v1alpha1.ClusterAccessConfig, inClient client.Client) (*clusterData, error) {
	clusterSecretName := cac.ClusterSecretRef.Name
	clusterSecretNamespace := cac.ClusterSecretRef.Namespace

	secret := &corev1.Secret{}
	errSecret := inClient.Get(ctx, types.NamespacedName{Name: clusterSecretName, Namespace: clusterSecretNamespace}, secret)
	if errSecret != nil {
		errClusterSecret := fmt.Errorf("failed to retrieve Cluster Secret Ref with name %s in namespace %s: %w", clusterSecretName, clusterSecretNamespace, errSecret)
		return nil, errClusterSecret
	}

	caData, ok := secret.Data[caDataKey]
	if !ok {
		return nil, fmt.Errorf("caData key %s not found in Secret '%s/%s'", caDataKey, clusterSecretNamespace, clusterSecretName)
	}

	audience, ok := secret.Data[audienceKey]
	if !ok {
		return nil, fmt.Errorf("audience key %s not found in Secret '%s/%s'", audienceKey, clusterSecretNamespace, clusterSecretName)
	}

	host, ok := secret.Data[hostKey]
	if !ok {
		return nil, fmt.Errorf("host key %s not found in Secret '%s/%s'", audienceKey, clusterSecretNamespace, clusterSecretName)
	}

	clsData := clusterData{
		caData:   string(caData),
		audience: string(audience),
		host:     string(host),
	}

	return &clsData, nil
}

type clusterData struct {
	caData   string
	audience string
	host     string
}

// CreateExternalQueryConfigSet creates a set of external query configs from a federated cluster access reference
func CreateExternalQueryConfigSet(ctx context.Context, fcaRef v1alpha1.FederateClusterAccessRef, inClient client.Client, restConfig *rest.Config) ([]orchestrator.QueryConfig, error) {

	rcaSetName := fcaRef.Name
	rcaSetNamespace := fcaRef.Namespace

	set := &v1alpha1.FederatedClusterAccess{}
	errSet := inClient.Get(ctx, types.NamespacedName{Name: rcaSetName, Namespace: rcaSetNamespace}, set)
	if errSet != nil {
		errRCA := fmt.Errorf("failed to retrieve federated cluster access with name %s in namespace %s: %w", rcaSetName, rcaSetNamespace, errSet)
		return nil, errRCA
	}

	kcPath := set.Spec.KubeConfigPath

	var options = metav1.ListOptions{}

	discoveryCli, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}
	gvr, err := orchestrator.GetGVRfromGVK(set.Spec.Target.GVK(), discoveryCli)
	if err != nil {
		return nil, err
	}

	dynamicClient, errCli := dynamic.NewForConfig(restConfig)
	if errCli != nil {
		return nil, fmt.Errorf("could not create dynamic client: %w", errCli)
	}

	list, err := dynamicClient.Resource(gvr).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", set.Spec.Target.GVK().String(), err)
	}

	return extractKubeConfigs(kcPath, list)

}

func extractKubeConfigs(kcPath string, list *unstructured.UnstructuredList) ([]orchestrator.QueryConfig, error) {
	queryConfigs := make([]orchestrator.QueryConfig, 0, len(list.Items))

	// TODO: not all resources will have kubeconfig data, need to handle this case

	// TODO: need to ad logging here
	for _, obj := range list.Items {

		fields := strings.Split(kcPath, ".")

		kubeconfigData, err := getKubeconfigAsBytes(&obj, fields...)

		if err != nil {
			// not found or an error happened
			continue
			// return nil, fmt.Errorf("could not find kubeconfig data in resource")
		}

		// Create a config from the kubeconfig data
		config, errRest := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
		if errRest != nil {
			return nil, fmt.Errorf("failed to create config from kubeconfig: %w", err)
		}

		kubeconfig, errKC := clientcmd.Load(kubeconfigData)
		if errKC != nil {
			return nil, fmt.Errorf("failed to load Config object from kubeconfigData: %w", errKC)
		}

		clusterName, err := extractHostName(kubeconfig.Clusters[kubeconfig.CurrentContext].Server)
		if err != nil {
			return nil, fmt.Errorf("failed to extract hostname from kubeconfig: %w", err)
		}

		// Create the client
		externalClient, err := client.New(config, client.Options{Scheme: externalScheme})
		if err != nil {
			return nil, fmt.Errorf("failed to create external client query config: %w", err)
		}

		queryConfigs = append(queryConfigs, orchestrator.QueryConfig{Client: externalClient, RestConfig: *config, ClusterName: &clusterName})

	}

	return queryConfigs, nil

}

func extractHostName(server string) (string, error) {
	// Parse the URL to get the hostname
	parsedURL, err := url.Parse(server)
	if err != nil {
		return "", fmt.Errorf("error parsing server URL: %w", err)
	}

	// Extract the hostname
	hostname := parsedURL.Hostname()

	// Remove the top-level domain if present
	parts := strings.Split(hostname, ".")
	if len(parts) > 1 && !isIP(hostname) {
		hostname = strings.Join(parts[:len(parts)-1], ".")
	}

	return hostname, nil
}

func isIP(host string) bool {
	return strings.Count(host, ".") == 3 && strings.IndexFunc(host, func(r rune) bool {
		return r != '.' && (r < '0' || r > '9')
	}) == -1
}

func getKubeconfigAsBytes(obj *unstructured.Unstructured, fields ...string) ([]byte, error) {
	kubeconfig, found, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	if err != nil {
		return nil, fmt.Errorf("error getting nested field: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("kubeconfig field not found")
	}
	// if string
	// return []byte(kubeconfig.(string)), nil

	// if otherting
	return json.Marshal(kubeconfig)
}
