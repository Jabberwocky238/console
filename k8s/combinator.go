package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type RDBItem struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Type string `json:"rdb_type"`
}

type KVItem struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Type string `json:"kv_type"`
}

type Combinator struct {
	UserUID string    `json:"-"`
	RDBs    []RDBItem `json:"rdb"`
	KVs     []KVItem  `json:"kv"`
}

// GetCombinatorConfig gets the combinator config for a user
func GetCombinatorConfig(userUID string) (*Combinator, error) {
	if K8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	configMapName := fmt.Sprintf("combinator-config-%s", userUID)

	cm, err := K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	configJSON := cm.Data["config.json"]
	c := &Combinator{UserUID: userUID}
	if err := c.FromJSON([]byte(configJSON)); err != nil {
		return nil, err
	}

	// Auto-detect types from URL
	for i := range c.RDBs {
		c.RDBs[i].Type = detectRDBType(c.RDBs[i].URL)
	}
	for i := range c.KVs {
		c.KVs[i].Type = detectKVType(c.KVs[i].URL)
	}

	return c, nil
}

// detectRDBType detects RDB type from URL prefix
func detectRDBType(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		return "postgres"
	case strings.HasPrefix(lower, "mysql://"):
		return "mysql"
	case strings.HasPrefix(lower, "sqlite://"), strings.HasPrefix(lower, "file:"):
		return "sqlite"
	default:
		return "unknown"
	}
}

// detectKVType detects KV type from URL prefix
func detectKVType(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasPrefix(lower, "redis://"), strings.HasPrefix(lower, "rediss://"):
		return "redis"
	case strings.HasPrefix(lower, "memory://"):
		return "memory"
	default:
		return "unknown"
	}
}

func (c *Combinator) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

func (c *Combinator) FromJSON(data []byte) error {
	return json.Unmarshal(data, c)
}

func (c *Combinator) Name() string {
	return fmt.Sprintf("combinator-%s", c.UserUID)
}

func (c *Combinator) ConfigMapName() string {
	return fmt.Sprintf("combinator-config-%s", c.UserUID)
}

// UpdateConfig updates ConfigMap for Combinator pod
func (c *Combinator) UpdateConfig() error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	configJSON, err := c.ToJSON()
	if err != nil {
		return err
	}

	ctx := context.Background()
	cm, err := K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Get(ctx, c.ConfigMapName(), metav1.GetOptions{})
	if err != nil {
		// Create new ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.ConfigMapName(),
				Namespace: CombinatorNamespace,
			},
			Data: map[string]string{
				"config.json": string(configJSON),
			},
		}
		_, err = K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	// Pod exists, call /reload API first
	if err := c.reloadConfig(configJSON); err != nil {
		return fmt.Errorf("reload failed: %w", err)
	}

	cm.Data["config.json"] = string(configJSON)
	_, err = K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// reloadConfig calls the combinator's /reload API
func (c *Combinator) reloadConfig(configJSON []byte) error {
	reloadURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8899/reload", c.Name(), CombinatorNamespace)

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

// buildPodSpec builds the Pod specification
func (c *Combinator) buildPodSpec() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name(),
			Namespace: CombinatorNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": c.UserUID,
			},
		},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{
				"project": "combinator-affinitive",
			},
			Containers: []corev1.Container{{
				Name:            "combinator",
				Image:           "ghcr.io/jabberwocky238/combinator:latest",
				ImagePullPolicy: corev1.PullAlways,
				Ports:           []corev1.ContainerPort{{ContainerPort: 8899, Name: "http"}},
				Args: []string{
					"start", "-c", "/config/config.json",
					"-l", "0.0.0.0:8899",
					"--watch", "all", "--watch-interval", "60",
				},
				Env: []corev1.EnvVar{{Name: "USER_UID", Value: c.UserUID}},
				VolumeMounts: []corev1.VolumeMount{{
					Name: "config", MountPath: "/config", ReadOnly: true,
				}},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8899)},
					},
					InitialDelaySeconds: 10, PeriodSeconds: 10,
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8899)},
					},
					InitialDelaySeconds: 5, PeriodSeconds: 5,
				},
			}},
			Volumes: []corev1.Volume{{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: c.ConfigMapName()},
					},
				},
			}},
			RestartPolicy: corev1.RestartPolicyAlways,
		},
	}
}

// Exists checks if combinator pod exists
func (c *Combinator) Exists() (bool, error) {
	if K8sClient == nil {
		return false, fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	_, err := K8sClient.CoreV1().Pods(CombinatorNamespace).Get(ctx, c.Name(), metav1.GetOptions{})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// Deploy creates combinator pod and related resources
func (c *Combinator) Deploy() error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()

	// Create ConfigMap first
	if err := c.UpdateConfig(); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// Create Pod
	pod := c.buildPodSpec()
	_, err := K8sClient.CoreV1().Pods(CombinatorNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	// Create Service
	if err := c.createService(ctx); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Create ExternalName Service
	if err := c.createExternalService(ctx); err != nil {
		return fmt.Errorf("failed to create external service: %w", err)
	}

	// Create IngressRoute
	if err := c.createIngressRoute(ctx); err != nil {
		return fmt.Errorf("failed to create ingress route: %w", err)
	}

	return nil
}

// Delete deletes combinator pod and related resources
func (c *Combinator) Delete() error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()

	// Delete Pod
	K8sClient.CoreV1().Pods(CombinatorNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})

	// Delete ConfigMap
	K8sClient.CoreV1().ConfigMaps(CombinatorNamespace).Delete(ctx, c.ConfigMapName(), metav1.DeleteOptions{})

	// Delete Services
	K8sClient.CoreV1().Services(CombinatorNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})
	K8sClient.CoreV1().Services(IngressNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})

	// Delete IngressRoute
	c.deleteIngressRoute(ctx)

	return nil
}

// createService creates a Service for the combinator pod
func (c *Combinator) createService(ctx context.Context) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name(),
			Namespace: CombinatorNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": c.UserUID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":      "combinator",
				"user-uid": c.UserUID,
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

// createExternalService creates an ExternalName Service in ingress namespace
func (c *Combinator) createExternalService(ctx context.Context) error {
	targetService := fmt.Sprintf("%s.%s.svc.cluster.local", c.Name(), CombinatorNamespace)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name(),
			Namespace: IngressNamespace,
			Labels: map[string]string{
				"app":      "combinator",
				"user-uid": c.UserUID,
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

// createIngressRoute creates an IngressRoute in ingress namespace
func (c *Combinator) createIngressRoute(ctx context.Context) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	ingressRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      c.Name(),
				"namespace": IngressNamespace,
				"labels": map[string]any{
					"app":      "combinator",
					"user-uid": c.UserUID,
				},
			},
			"spec": map[string]any{
				"entryPoints": []any{"websecure"},
				"routes": []any{
					map[string]any{
						"match": fmt.Sprintf("Host(`%s.combinator.%s`)", c.UserUID, Domain),
						"kind":  "Rule",
						"services": []any{
							map[string]any{
								"name": c.Name(),
								"port": 8899,
							},
						},
					},
				},
				"tls": map[string]any{
					"secretName": "ingress-tls",
				},
			},
		},
	}

	_, err := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace).Create(ctx, ingressRoute, metav1.CreateOptions{})
	return err
}

// deleteIngressRoute deletes an IngressRoute
func (c *Combinator) deleteIngressRoute(ctx context.Context) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}
	return DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})
}
