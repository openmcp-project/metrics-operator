package config

import (
	"context"
	"fmt"
	"net/url"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	orc "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/metric_orchestratorV2"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	caDataKey   = "caData"
	audienceKey = "audience"
	hostKey     = "host"
)

var (
	externalScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(externalScheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(externalScheme))
	utilruntime.Must(businessv1.AddToScheme(externalScheme))

}

func CreateExternalQueryConfig(ctx context.Context, racRef *businessv1.RemoteClusterAccessRef, inClient client.Client) (*orc.QueryConfig, error) {

	rcaName := racRef.Name
	rcaNamespace := racRef.Namespace

	rca := &businessv1.RemoteClusterAccess{}
	err := inClient.Get(ctx, types.NamespacedName{Name: rcaName, Namespace: rcaNamespace}, rca)
	if err != nil {
		errRCA := fmt.Errorf("failed to retrieve Remote Cluster Acces Ref with name %s in namespace %s: %v", rcaName, rcaNamespace, err)
		return nil, errRCA
	}

	kcRef := rca.Spec.KubeConfigSecretRef
	if kcRef != nil {
		return queryConfigFromKubeConfig(kcRef, inClient, externalScheme)
	}

	cac := rca.Spec.ClusterAccessConfig
	if cac != nil {
		return queryConfigFromClusterAccessConfig(ctx, cac, inClient, externalScheme)
	}

	return nil, fmt.Errorf("kubeconfigSecretRef and clusterAccessConfig are both nil")
}

func queryConfigFromClusterAccessConfig(ctx context.Context, cac *businessv1.ClusterAccessConfig, inClient client.Client, externalScheme *runtime.Scheme) (*orc.QueryConfig, error) {
	clsData, errData := getCusterDataFromSecret(ctx, cac, inClient)
	if errData != nil {
		return nil, errData
	}

	saName := cac.ServiceAccountName
	saNamespace := cac.ServiceAccountNamespace

	token, errToken := getTokenWithAPI(inClient, saName, saNamespace, clsData.audience)
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
		return nil, fmt.Errorf("failed to create external client: %v", err)

	}

	parsedHost, errParse := url.Parse(clsData.host)
	if errParse != nil {
		return nil, fmt.Errorf("failed to parse host URL: %v", errParse)
	}
	hostName := parsedHost.Hostname()

	return &orc.QueryConfig{Client: externalClient, RestConfig: *restConfig, ClusterName: &hostName}, nil
}

func queryConfigFromKubeConfig(kcRef *businessv1.KubeConfigSecretRef, inClient client.Client, externalScheme *runtime.Scheme) (*orc.QueryConfig, error) {
	secretName := kcRef.SecretReference.Name
	secretNamespace := kcRef.SecretReference.Namespace

	// Retrieve the Secret
	secret := &corev1.Secret{}
	err := inClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		errSecret := fmt.Errorf("failed to retrieve KubeConfig Secret Ref with name %s in namespace %s: %v", secretName, secretNamespace, err)
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
		return nil, fmt.Errorf("failed to create config from kubeconfig: %v", err)
	}

	kubeconfig, errKC := clientcmd.Load(kubeconfigData)
	if errKC != nil {
		return nil, fmt.Errorf("failed to load Config object from kubeconfigData: %v", errKC)
	}

	clusterName := kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster

	// Create the client
	externalClient, err := client.New(config, client.Options{Scheme: externalScheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &orc.QueryConfig{Client: externalClient, RestConfig: *config, ClusterName: &clusterName}, nil
}

func getTokenWithAPI(inClient client.Client, serviceAccount string, namespace, audience string) (string, error) {
	tm, errTM := GetTokenManager(inClient)

	if errTM != nil {
		return "", fmt.Errorf("failed to get token manager: %v", errTM)
	}

	token, errTK := tm.GetToken(namespace, serviceAccount, audience)

	if errTK != nil {
		return "", fmt.Errorf("failed to get token for %s/%s/%s: %v", namespace, serviceAccount, audience, errTK)
	}

	return token, nil
}

func getCusterDataFromSecret(ctx context.Context, cac *businessv1.ClusterAccessConfig, inClient client.Client) (*clusterData, error) {
	clusterSecretName := cac.ClusterSecretRef.Name
	clusterSecretNamespace := cac.ClusterSecretRef.Namespace

	secret := &corev1.Secret{}
	errSecret := inClient.Get(ctx, types.NamespacedName{Name: clusterSecretName, Namespace: clusterSecretNamespace}, secret)
	if errSecret != nil {
		errClusterSecret := fmt.Errorf("failed to retrieve Cluster Secret Ref with name %s in namespace %s: %v", clusterSecretName, clusterSecretNamespace, errSecret)
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
