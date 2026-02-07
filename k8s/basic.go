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
	RestConfig          *rest.Config
	Domain              string
	Namespace           = "console"    // Control plane namespace
	CombinatorNamespace = "combinator" // Combinator pods namespace
	IngressNamespace    = "ingress"    // Ingress namespace
	WorkerNamespace     = "worker"     // Worker namespace

	RDBNamespace        = "cockroachdb"
	CockroachDBHost     = "cockroachdb-public.cockroachdb.svc.cluster.local"
	CockroachDBPort     = "26257"
	CockroachDBAdminDSN = "postgresql://root@cockroachdb-public.cockroachdb.svc.cluster.local:26257?sslmode=disable"

	RDBManager *RootRDBManager
)

var IngressRouteGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "ingressroutes",
}

var certificateGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
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

	RestConfig = config

	K8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	DynamicClient, err = dynamic.NewForConfig(config)
	return err
}
