package k8s

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	K8sClient           *kubernetes.Clientset
	DynamicClient       dynamic.Interface
	Domain              string
	Namespace           = "console" // Control plane namespace
	CombinatorNamespace = "combinator" // Combinator pods namespace
	IngressNamespace    = "ingress"    // Ingress namespace
	WorkerNamespace     = "worker"     // Worker namespace
)

var ingressRouteGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "ingressroutes",
}

// InitK8s initializes Kubernetes client
func InitK8s(kubeconfig string) error {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		// In-cluster config
		config, err = rest.InClusterConfig()
	} else {
		// Out-of-cluster config
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return err
	}

	K8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	DynamicClient, err = dynamic.NewForConfig(config)
	return err
}
