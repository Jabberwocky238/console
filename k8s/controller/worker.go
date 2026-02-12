package controller

import (
	"context"
	"fmt"
	"strings"

	"jabberwocky238/console/k8s"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WorkerName returns the canonical resource name for a worker.
func WorkerName(workerID, ownerID string) string {
	return fmt.Sprintf("w-%s-%s", workerID, ownerID)
}

// Name returns the worker's resource name
func (w *WorkerAppSpec) Name() string {
	return WorkerName(w.WorkerID, w.OwnerID)
}

func (w *WorkerAppSpec) Labels() map[string]string {
	return map[string]string{
		"app":       w.Name(),
		"worker-id": w.WorkerID,
		"owner-id":  w.OwnerID,
	}
}

func (w *WorkerAppSpec) EnvConfigMapName() string {
	return fmt.Sprintf("%s-env", w.Name())
}

func (w *WorkerAppSpec) SecretName() string {
	return fmt.Sprintf("%s-secret", w.Name())
}

func (w *WorkerAppSpec) CombinatorEndpoint() string {
	return fmt.Sprintf("http://combinator.%s.svc.cluster.local:8899", k8s.CombinatorNamespace)
}
func (w *WorkerAppSpec) EnsureDeployment(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name(),
			Namespace: k8s.WorkerNamespace,
			Labels:    w.Labels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": w.Name()},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: w.Labels()},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
								Weight: 100,
								PodAffinityTerm: corev1.PodAffinityTerm{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": "combinator",
										},
									},
									Namespaces:  []string{k8s.CombinatorNamespace},
									TopologyKey: "kubernetes.io/hostname",
								},
							}},
						},
					},
					Containers: []corev1.Container{{
						Name:  w.Name(),
						Image: w.Image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: int32(w.Port),
						}},
						Env: []corev1.EnvVar{
							{Name: "COMBINATOR_API_ENDPOINT", Value: w.CombinatorEndpoint()},
							{Name: "RAYSAIL_UID", Value: w.OwnerID},
							{Name: "RAYSAIL_SECRET_KEY", Value: w.OwnerSK},
						},
						EnvFrom: []corev1.EnvFromSource{
							{
								ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: w.EnvConfigMapName()},
								},
							},
							{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: w.SecretName()},
								},
							},
						},
					}},
				},
			},
		},
	}

	client := k8s.K8sClient.AppsV1().Deployments(k8s.WorkerNamespace)
	_, err := client.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, deployment, metav1.CreateOptions{})
	} else if err == nil {
		_, err = client.Update(ctx, deployment, metav1.UpdateOptions{})
	}
	return err
}

// EnsureService checks and creates the Service if missing.
func (w *WorkerAppSpec) EnsureService(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name(),
			Namespace: k8s.WorkerNamespace,
			Labels:    w.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": w.Name()},
			Ports: []corev1.ServicePort{{
				Port:     int32(w.Port),
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	client := k8s.K8sClient.CoreV1().Services(k8s.WorkerNamespace)
	_, err := client.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, service, metav1.CreateOptions{})
	}
	return err
}

// EnsureConfigMap ensures the worker's env ConfigMap exists (empty if not present).
func (w *WorkerAppSpec) EnsureConfigMap(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	client := k8s.K8sClient.CoreV1().ConfigMaps(k8s.WorkerNamespace)
	_, err := client.Get(ctx, w.EnvConfigMapName(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      w.EnvConfigMapName(),
				Namespace: k8s.WorkerNamespace,
				Labels:    w.Labels(),
			},
			Data: map[string]string{},
		}
		_, err = client.Create(ctx, cm, metav1.CreateOptions{})
	}
	return err
}

// EnsureSecret ensures the worker's Secret exists (empty if not present).
func (w *WorkerAppSpec) EnsureSecret(ctx context.Context) error {
	if k8s.K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}
	client := k8s.K8sClient.CoreV1().Secrets(k8s.WorkerNamespace)
	_, err := client.Get(ctx, w.SecretName(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      w.SecretName(),
				Namespace: k8s.WorkerNamespace,
				Labels:    w.Labels(),
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{},
		}
		_, err = client.Create(ctx, secret, metav1.CreateOptions{})
	}
	return err
}

// EnsureIngressRoute checks and creates/updates the IngressRoute if missing.
func (w *WorkerAppSpec) EnsureIngressRoute(ctx context.Context) error {
	if k8s.DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	host := fmt.Sprintf("%s-%s.worker.%s", w.WorkerID, w.OwnerID, k8s.Domain)

	ingressRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      w.Name(),
				"namespace": k8s.IngressNamespace,
				"labels": map[string]any{
					"app":       w.Name(),
					"worker-id": w.WorkerID,
					"owner-id":  w.OwnerID,
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
								"name":      w.Name(),
								"namespace": k8s.WorkerNamespace,
								"port":      w.Port,
							},
						},
					},
				},
				"tls": map[string]any{
					"secretName": "worker-tls",
				},
			},
		},
	}

	client := k8s.DynamicClient.Resource(k8s.IngressRouteGVR).Namespace(k8s.IngressNamespace)
	existing, err := client.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, ingressRoute, metav1.CreateOptions{})
	} else if err == nil {
		ingressRoute.SetResourceVersion(existing.GetResourceVersion())
		_, err = client.Update(ctx, ingressRoute, metav1.UpdateOptions{})
	}
	return err
}

// DeleteAll deletes all sub-resources for this worker.
func (w *WorkerAppSpec) DeleteAll(ctx context.Context) {
	if k8s.K8sClient != nil {
		k8s.K8sClient.AppsV1().Deployments(k8s.WorkerNamespace).Delete(ctx, w.Name(), metav1.DeleteOptions{})
		k8s.K8sClient.CoreV1().Services(k8s.WorkerNamespace).Delete(ctx, w.Name(), metav1.DeleteOptions{})
		k8s.K8sClient.CoreV1().ConfigMaps(k8s.WorkerNamespace).Delete(ctx, w.EnvConfigMapName(), metav1.DeleteOptions{})
		k8s.K8sClient.CoreV1().Secrets(k8s.WorkerNamespace).Delete(ctx, w.SecretName(), metav1.DeleteOptions{})
	}
	if k8s.DynamicClient != nil {
		k8s.DynamicClient.Resource(k8s.IngressRouteGVR).Namespace(k8s.IngressNamespace).Delete(ctx, w.Name(), metav1.DeleteOptions{})
	}
}

// ListWorkers lists all workers by querying Deployments with label selectors.
func ListWorkers(workerId string, ownerId string) ([]WorkerAppSpec, error) {
	if k8s.K8sClient == nil {
		return nil, fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	opts := metav1.ListOptions{}

	var selectors []string
	if workerId != "" {
		selectors = append(selectors, fmt.Sprintf("worker-id=%s", workerId))
	}
	if ownerId != "" {
		selectors = append(selectors, fmt.Sprintf("owner-id=%s", ownerId))
	}
	opts.LabelSelector = strings.Join(selectors, ",")

	deployments, err := k8s.K8sClient.AppsV1().Deployments(k8s.WorkerNamespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}

	var workers []WorkerAppSpec
	for _, d := range deployments.Items {
		workers = append(workers, WorkerAppSpec{
			WorkerID: d.Labels["worker-id"],
			OwnerID:  d.Labels["owner-id"],
			Image:    d.Spec.Template.Spec.Containers[0].Image,
		})
	}
	return workers, nil
}
