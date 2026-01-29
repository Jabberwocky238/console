package main

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	K8sClient *kubernetes.Clientset
	Namespace = "storebirth"
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
	cm, err := K8sClient.CoreV1().ConfigMaps(Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: Namespace,
			},
			Data: map[string]string{
				"config.json": string(configJSON),
			},
		}
		_, err = K8sClient.CoreV1().ConfigMaps(Namespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	// Update existing ConfigMap
	cm.Data["config.json"] = string(configJSON)
	_, err = K8sClient.CoreV1().ConfigMaps(Namespace).Update(ctx, cm, metav1.UpdateOptions{})
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

	_, err := K8sClient.CoreV1().Pods(Namespace).Get(ctx, podName, metav1.GetOptions{})
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
			Namespace: Namespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": userUID,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "combinator",
					Image: "combinator:latest",
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

	_, err := K8sClient.CoreV1().Pods(Namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
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
	err := K8sClient.CoreV1().Pods(Namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	// Delete ConfigMap
	err = K8sClient.CoreV1().ConfigMaps(Namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete configmap: %w", err)
	}

	return nil
}
