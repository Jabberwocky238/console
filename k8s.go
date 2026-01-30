package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	K8sClient           *kubernetes.Clientset
	DynamicClient       dynamic.Interface
	Namespace           = "storebirth" // Control plane namespace
	CombinatorNamespace = "combinator" // Combinator pods namespace
	IngressNamespace    = "ingress"    // Ingress namespace
)

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

// UpdateUserConfig updates ConfigMap for user's combinator pod
func UpdateUserConfig(userUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Generate config
	config, err := generateConfig(userUID)
	if err != nil {
		return err
	}

	configJSON, _ := json.MarshalIndent(config, "", "  ")
	configMapName := fmt.Sprintf("combinator-config-%s", userUID)

	ctx := context.Background()
	cm, err := K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: CombinatorNamespace,
			},
			Data: map[string]string{
				"config.json": string(configJSON),
			},
		}
		_, err = K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	// Update existing ConfigMap
	cm.Data["config.json"] = string(configJSON)
	_, err = K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// generateConfig generates combinator config for user
func generateConfig(userUID string) (map[string]any, error) {
	// Get RDBs
	rdbRows, err := DB.Query(
		`SELECT uid, rdb_type, url FROM user_rdbs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1) AND enabled = true`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer rdbRows.Close()

	var rdbs []map[string]any
	for rdbRows.Next() {
		var uid, rdbType, url string
		rdbRows.Scan(&uid, &rdbType, &url)
		rdbs = append(rdbs, map[string]any{
			"id":      uid,
			"enabled": true,
			"url":     url,
		})
	}

	// Get KVs
	kvRows, err := DB.Query(
		`SELECT uid, kv_type, url FROM user_kvs
		 WHERE user_id = (SELECT id FROM users WHERE uid = $1) AND enabled = true`,
		userUID,
	)
	if err != nil {
		return nil, err
	}
	defer kvRows.Close()

	var kvs []map[string]any
	for kvRows.Next() {
		var uid, kvType, url string
		kvRows.Scan(&uid, &kvType, &url)
		kvs = append(kvs, map[string]any{
			"id":      uid,
			"enabled": true,
			"url":     url,
		})
	}

	return map[string]any{
		"rdb": rdbs,
		"kv":  kvs,
	}, nil
}

// CheckUserPodExists checks if a combinator pod exists for user
func CheckUserPodExists(userUID string) (bool, error) {
	if K8sClient == nil {
		return false, fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", userUID)

	_, err := K8sClient.CoreV1().Pods(CombinatorNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		// Pod doesn't exist
		return false, nil
	}
	return true, nil
}

// CreateUserPod creates a combinator pod for user
func CreateUserPod(userUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", userUID)
	configMapName := fmt.Sprintf("combinator-config-%s", userUID)

	// Create ConfigMap first
	if err := UpdateUserConfig(userUID); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// Create Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: CombinatorNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": userUID,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "combinator",
					Image: "ghcr.io/jabberwocky238/combinator:latest",
					Ports: []corev1.ContainerPort{
						{ContainerPort: 8899, Name: "http"},
					},
					Env: []corev1.EnvVar{
						{Name: "USER_UID", Value: userUID},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config",
							MountPath: "/config",
							ReadOnly:  true,
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: configMapName,
							},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}

	_, err := K8sClient.CoreV1().Pods(CombinatorNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	// Create Service for the pod
	if err := createCombinatorService(ctx, userUID); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Create ExternalName Service in ingress namespace
	if err := createCombinatorExternalService(ctx, userUID); err != nil {
		return fmt.Errorf("failed to create external service: %w", err)
	}

	// Create IngressRoute in ingress namespace
	if err := createCombinatorIngressRoute(ctx, userUID); err != nil {
		return fmt.Errorf("failed to create ingress route: %w", err)
	}

	return nil
}

// DeleteUserPod deletes a combinator pod for user
func DeleteUserPod(userUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", userUID)
	configMapName := fmt.Sprintf("combinator-config-%s", userUID)

	// Delete Pod
	err := K8sClient.CoreV1().Pods(CombinatorNamespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	// Delete ConfigMap
	err = K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete configmap: %w", err)
	}

	// Delete Service
	serviceName := fmt.Sprintf("combinator-%s", userUID)
	K8sClient.CoreV1().Services(CombinatorNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})

	// Delete ExternalName Service in ingress namespace
	K8sClient.CoreV1().Services(IngressNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})

	// Delete IngressRoute in ingress namespace
	deleteIngressRoute(ctx, userUID)

	return nil
}

// createCombinatorService creates a Service for the combinator pod
func createCombinatorService(ctx context.Context, userUID string) error {
	serviceName := fmt.Sprintf("combinator-%s", userUID)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: CombinatorNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": userUID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":      "combinator",
				"user-uid": userUID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Port:     8899,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	_, err := K8sClient.CoreV1().Services(CombinatorNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

// createCombinatorExternalService creates an ExternalName Service in ingress namespace
func createCombinatorExternalService(ctx context.Context, userUID string) error {
	serviceName := fmt.Sprintf("combinator-%s", userUID)
	targetService := fmt.Sprintf("combinator-%s.%s.svc.cluster.local", userUID, CombinatorNamespace)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: IngressNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": userUID,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: targetService,
		},
	}

	_, err := K8sClient.CoreV1().Services(IngressNamespace).Create(ctx, service, metav1.CreateOptions{})
	return err
}

// createCombinatorIngressRoute creates an IngressRoute in ingress namespace
func createCombinatorIngressRoute(ctx context.Context, userUID string) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	ingressRouteName := fmt.Sprintf("combinator-%s", userUID)
	serviceName := fmt.Sprintf("combinator-%s", userUID)

	// Get domain from environment variable
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		domain = "example.com" // fallback default
	}

	// Define IngressRoute GVR
	ingressRouteGVR := schema.GroupVersionResource{
		Group:    "traefik.io",
		Version:  "v1alpha1",
		Resource: "ingressroutes",
	}

	// Create IngressRoute object
	ingressRoute := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]interface{}{
				"name":      ingressRouteName,
				"namespace": IngressNamespace,
				"labels": map[string]interface{}{
					"app":      "combinator",
					"user-uid": userUID,
				},
			},
			"spec": map[string]interface{}{
				"entryPoints": []interface{}{"websecure"},
				"routes": []interface{}{
					map[string]interface{}{
						"match": fmt.Sprintf("Host(`%s.combinator.%s`)", userUID, domain),
						"kind":  "Rule",
						"services": []interface{}{
							map[string]interface{}{
								"name": serviceName,
								"port": 8899,
							},
						},
					},
				},
				"tls": map[string]interface{}{
					"secretName": "ingress-tls",
				},
			},
		},
	}

	_, err := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace).Create(ctx, ingressRoute, metav1.CreateOptions{})
	return err
}

// deleteIngressRoute deletes an IngressRoute in ingress namespace
func deleteIngressRoute(ctx context.Context, userUID string) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	ingressRouteName := fmt.Sprintf("combinator-%s", userUID)

	// Define IngressRoute GVR
	ingressRouteGVR := schema.GroupVersionResource{
		Group:    "traefik.io",
		Version:  "v1alpha1",
		Resource: "ingressroutes",
	}

	err := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace).Delete(ctx, ingressRouteName, metav1.DeleteOptions{})
	return err
}
