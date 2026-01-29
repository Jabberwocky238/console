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
	Namespace = "combinator"
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
func UpdateUserConfig(userUUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	// Generate config
	config, err := generateConfig(userUUID)
	if err != nil {
		return err
	}

	configJSON, _ := json.MarshalIndent(config, "", "  ")
	configMapName := fmt.Sprintf("combinator-config-%s", userUUID)

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
func generateConfig(userUUID string) (map[string]any, error) {
	// Get RDBs
	rdbRows, err := DB.Query(
		`SELECT uuid, rdb_type, url FROM user_rdbs
		 WHERE user_id = (SELECT id FROM users WHERE uuid = $1) AND enabled = true`,
		userUUID,
	)
	if err != nil {
		return nil, err
	}
	defer rdbRows.Close()

	var rdbs []map[string]any
	for rdbRows.Next() {
		var uuid, rdbType, url string
		rdbRows.Scan(&uuid, &rdbType, &url)
		rdbs = append(rdbs, map[string]any{
			"id":      uuid,
			"enabled": true,
			"url":     url,
		})
	}

	// Get KVs
	kvRows, err := DB.Query(
		`SELECT uuid, kv_type, url FROM user_kvs
		 WHERE user_id = (SELECT id FROM users WHERE uuid = $1) AND enabled = true`,
		userUUID,
	)
	if err != nil {
		return nil, err
	}
	defer kvRows.Close()

	var kvs []map[string]any
	for kvRows.Next() {
		var uuid, kvType, url string
		kvRows.Scan(&uuid, &kvType, &url)
		kvs = append(kvs, map[string]any{
			"id":      uuid,
			"enabled": true,
			"url":     url,
		})
	}

	return map[string]any{
		"rdb": rdbs,
		"kv":  kvs,
	}, nil
}

// CreateUserPod creates a combinator pod for user
func CreateUserPod(userUUID string) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	podName := fmt.Sprintf("combinator-%s", userUUID)
	configMapName := fmt.Sprintf("combinator-config-%s", userUUID)

	// Create ConfigMap first
	if err := UpdateUserConfig(userUUID); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// Create Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: Namespace,
			Labels: map[string]string{
				"app":       "combinator",
				"user-uuid": userUUID,
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
						{Name: "USER_UUID", Value: userUUID},
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
