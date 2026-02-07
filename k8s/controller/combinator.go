package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"jabberwocky238/console/k8s"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type RDBItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type KVItem struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Type string `json:"kv_type"`
}

type CombinatorConfig struct {
	RDBs []RDBItem `json:"rdb"`
	KVs  []KVItem  `json:"kv"`
}

type Combinator struct {
	UserUID string
	Config  string // JSON string of CombinatorConfig
}

func (c *Combinator) Name() string {
	return fmt.Sprintf("combinator-%s", c.UserUID)
}

func (c *Combinator) Labels() map[string]string {
	return map[string]string{
		"app":      c.Name(),
		"user-uid": c.UserUID,
	}
}

func (c *Combinator) ConfigMapName() string {
	return fmt.Sprintf("combinator-config-%s", c.UserUID)
}

// ParseConfig parses the JSON config string into CombinatorConfig
func (c *Combinator) ParseConfig() (*CombinatorConfig, error) {
	cfg := &CombinatorConfig{}
	if c.Config == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(c.Config), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}


// EnsureConfigMap ensures the ConfigMap exists with the correct config
func (c *Combinator) EnsureConfigMap(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.ConfigMapName(),
			Namespace: k8s.CombinatorNamespace,
			Labels:    c.Labels(),
		},
		Data: map[string]string{
			"config.json": c.Config,
		},
	}

	client := k8s.K8sClient.CoreV1().ConfigMaps(k8s.CombinatorNamespace)
	existing, err := client.Get(ctx, c.ConfigMapName(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, cm, metav1.CreateOptions{})
		return err
	} else if err != nil {
		return err
	}

	existing.Data = cm.Data
	existing.Labels = cm.Labels
	_, err = client.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// EnsureDeployment ensures the Deployment exists
func (c *Combinator) EnsureDeployment(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	replicas := int32(1)
	labels := c.Labels()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name(),
			Namespace: k8s.CombinatorNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": c.Name()}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       c.buildPodSpec(),
			},
		},
	}

	client := k8s.K8sClient.AppsV1().Deployments(k8s.CombinatorNamespace)
	_, err := client.Get(ctx, c.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, deployment, metav1.CreateOptions{})
	} else if err == nil {
		_, err = client.Update(ctx, deployment, metav1.UpdateOptions{})
	}
	return err
}

func (c *Combinator) buildPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
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
	}
}

// EnsureService ensures the Service exists
func (c *Combinator) EnsureService(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name(),
			Namespace: k8s.CombinatorNamespace,
			Labels:    c.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": c.Name()},
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Port:     8899,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	client := k8s.K8sClient.CoreV1().Services(k8s.CombinatorNamespace)
	_, err := client.Get(ctx, c.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, service, metav1.CreateOptions{})
	}
	return err
}

// EnsureIngressRoute ensures the IngressRoute exists
func (c *Combinator) EnsureIngressRoute(ctx context.Context) error {
	if k8s.DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	host := fmt.Sprintf("%s.combinator.%s", c.UserUID, k8s.Domain)

	ingressRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      c.Name(),
				"namespace": k8s.IngressNamespace,
				"labels": map[string]any{
					"app":      c.Name(),
					"user-uid": c.UserUID,
				},
			},
			"spec": map[string]any{
				"entryPoints": []any{"websecure"},
				"routes": []any{
					map[string]any{
						"match": fmt.Sprintf("Host(`%s`)", host),
						"kind":  "Rule",
						"services": []any{
							map[string]any{
								"name":      c.Name(),
								"namespace": k8s.CombinatorNamespace,
								"port":      8899,
							},
						},
					},
				},
				"tls": map[string]any{
					"secretName": "combinator-tls",
				},
			},
		},
	}

	client := k8s.DynamicClient.Resource(k8s.IngressRouteGVR).Namespace(k8s.IngressNamespace)
	_, err := client.Get(ctx, c.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, ingressRoute, metav1.CreateOptions{})
	} else if err == nil {
		_, err = client.Update(ctx, ingressRoute, metav1.UpdateOptions{})
	}
	return err
}

// DeleteAll deletes all sub-resources for this combinator
func (c *Combinator) DeleteAll(ctx context.Context) {
	if k8s.K8sClient != nil {
		k8s.K8sClient.AppsV1().Deployments(k8s.CombinatorNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})
		k8s.K8sClient.CoreV1().Services(k8s.CombinatorNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})
		k8s.K8sClient.CoreV1().ConfigMaps(k8s.CombinatorNamespace).Delete(ctx, c.ConfigMapName(), metav1.DeleteOptions{})
	}
	if k8s.DynamicClient != nil {
		k8s.DynamicClient.Resource(k8s.IngressRouteGVR).Namespace(k8s.IngressNamespace).Delete(ctx, c.Name(), metav1.DeleteOptions{})
	}
}

// ReloadConfig calls the combinator's /reload API
func (c *Combinator) ReloadConfig(configJSON []byte) error {
	reloadURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8899/reload", c.Name(), k8s.CombinatorNamespace)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", reloadURL, bytes.NewReader(configJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
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
