package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"jabberwocky238/console/dblayer"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// UpdateCombinatorConfig updates ConfigMap for Combinator's combinator pod
func UpdateCombinatorConfig(userUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Generate config
	config, err := generateCombinatorConfig(userUID)
	if err != nil {
		return err
	}

	configJSON, _ := json.MarshalIndent(config, "", "  ")
	configMapName := fmt.Sprintf("combinator-config-%s", userUID)

	ctx := context.Background()
	cm, err := K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap (pod not created yet, no need to reload)
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

	// Pod exists, call /reload API first to validate config
	if err := reloadCombinatorConfig(userUID, configJSON); err != nil {
		return fmt.Errorf("reload failed: %w", err)
	}

	// Reload succeeded, persist to ConfigMap
	cm.Data["config.json"] = string(configJSON)
	_, err = K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// generateConfig generates combinator config for Combinator
func generateCombinatorConfig(userUID string) (map[string]any, error) {
	// Get RDBs
	rdbItems, err := dblayer.GetUserRDBsForConfig(userUID)
	if err != nil {
		return nil, err
	}

	var rdbs []map[string]any
	for _, item := range rdbItems {
		rdbs = append(rdbs, map[string]any{
			"id":      item.UID,
			"enabled": true,
			"url":     item.URL,
		})
	}

	// Get KVs
	kvItems, err := dblayer.GetUserKVsForConfig(userUID)
	if err != nil {
		return nil, err
	}

	var kvs []map[string]any
	for _, item := range kvItems {
		kvs = append(kvs, map[string]any{
			"id":      item.UID,
			"enabled": true,
			"url":     item.URL,
		})
	}

	return map[string]any{
		"rdb": rdbs,
		"kv":  kvs,
	}, nil
}

// reloadCombinatorConfig calls the combinator's /reload API to validate and apply config
func reloadCombinatorConfig(userUID string, configJSON []byte) error {
	reloadURL := fmt.Sprintf("http://combinator-%s.%s.svc.cluster.local:8899/reload", userUID, CombinatorNamespace)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", reloadURL, bytes.NewReader(configJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reload returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CheckCombinatorPodExists checks if a combinator pod exists for Combinator
func CheckCombinatorPodExists(CombinatorUID string) (bool, error) {
	if K8sClient == nil {
		return false, fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", CombinatorUID)

	_, err := K8sClient.CoreV1().Pods(CombinatorNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		// Pod doesn't exist
		return false, nil
	}
	return true, nil
}

// CreateCombinatorPod creates a combinator pod for Combinator
func CreateCombinatorPod(CombinatorUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", CombinatorUID)
	configMapName := fmt.Sprintf("combinator-config-%s", CombinatorUID)

	// Create ConfigMap first
	if err := UpdateCombinatorConfig(CombinatorUID); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// Create Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: CombinatorNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": CombinatorUID,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "combinator",
					Image:           "ghcr.io/jabberwocky238/combinator:latest",
					ImagePullPolicy: corev1.PullAlways,
					Ports: []corev1.ContainerPort{
						{ContainerPort: 8899, Name: "http"},
					},
					Args: []string{
						"start",
						"-c",
						"/config/config.json",
						"-l",
						"0.0.0.0:8899",
						"--watch",
						"all",
						"--watch-interval",
						"60",
					},
					Env: []corev1.EnvVar{
						{Name: "USER_UID", Value: CombinatorUID},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config",
							MountPath: "/config",
							ReadOnly:  true,
						},
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(8899),
							},
						},
						InitialDelaySeconds: 10,
						PeriodSeconds:       10,
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromInt(8899),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
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
	if err := createCombinatorService(ctx, CombinatorUID); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Create ExternalName Service in ingress namespace
	if err := createCombinatorExternalService(ctx, CombinatorUID); err != nil {
		return fmt.Errorf("failed to create external service: %w", err)
	}

	// Create IngressRoute in ingress namespace
	if err := createCombinatorIngressRoute(ctx, CombinatorUID); err != nil {
		return fmt.Errorf("failed to create ingress route: %w", err)
	}

	return nil
}

// DeleteCombinatorPod deletes a combinator pod for Combinator
func DeleteCombinatorPod(CombinatorUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", CombinatorUID)
	configMapName := fmt.Sprintf("combinator-config-%s", CombinatorUID)

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
	serviceName := fmt.Sprintf("combinator-%s", CombinatorUID)
	K8sClient.CoreV1().Services(CombinatorNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})

	// Delete ExternalName Service in ingress namespace
	K8sClient.CoreV1().Services(IngressNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{})

	// Delete IngressRoute in ingress namespace
	deleteIngressRoute(ctx, CombinatorUID)

	return nil
}

// createCombinatorService creates a Service for the combinator pod
func createCombinatorService(ctx context.Context, CombinatorUID string) error {
	serviceName := fmt.Sprintf("combinator-%s", CombinatorUID)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: CombinatorNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": CombinatorUID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":      "combinator",
				"user-uid": CombinatorUID,
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
func createCombinatorExternalService(ctx context.Context, CombinatorUID string) error {
	serviceName := fmt.Sprintf("combinator-%s", CombinatorUID)
	targetService := fmt.Sprintf("combinator-%s.%s.svc.cluster.local", CombinatorUID, CombinatorNamespace)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: IngressNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": CombinatorUID,
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
func createCombinatorIngressRoute(ctx context.Context, CombinatorUID string) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	ingressRouteName := fmt.Sprintf("combinator-%s", CombinatorUID)
	serviceName := fmt.Sprintf("combinator-%s", CombinatorUID)

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
					"user-uid": CombinatorUID,
				},
			},
			"spec": map[string]interface{}{
				"entryPoints": []interface{}{"websecure"},
				"routes": []interface{}{
					map[string]interface{}{
						"match": fmt.Sprintf("Host(`%s.combinator.%s`)", CombinatorUID, Domain),
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
func deleteIngressRoute(ctx context.Context, CombinatorUID string) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	ingressRouteName := fmt.Sprintf("combinator-%s", CombinatorUID)

	err := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace).Delete(ctx, ingressRouteName, metav1.DeleteOptions{})
	return err
}
