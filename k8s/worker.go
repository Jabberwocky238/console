package k8s

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Worker represents a worker deployment
type Worker struct {
	WorkerID string `json:"worker_id"`
	OwnerID  string `json:"owner_id"`
	Image    string `json:"image"`
	Port     int    `json:"port"`
}

// Name returns the worker's resource name
func (w *Worker) Name() string {
	return fmt.Sprintf("%s-%s", w.WorkerID, w.OwnerID)
}

func (w *Worker) Labels() map[string]string {
	return map[string]string{
		"app":       w.Name(),
		"worker-id": w.WorkerID,
		"owner-id":  w.OwnerID,
	}
}

// DeployWorker deploys a worker to K8s
func (w *Worker) Deploy() error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()

	if err := w.deployWorkerDeployment(ctx); err != nil {
		return fmt.Errorf("deploy deployment failed: %w", err)
	}
	if err := w.deployWorkerService(ctx); err != nil {
		return fmt.Errorf("deploy service failed: %w", err)
	}
	if err := w.deployWorkerExternalService(ctx); err != nil {
		return fmt.Errorf("deploy external service failed: %w", err)
	}
	if err := w.deployWorkerIngressRoute(ctx); err != nil {
		return fmt.Errorf("deploy ingress route failed: %w", err)
	}
	return nil
}

// DeleteWorker deletes a worker from K8s
func (w *Worker) Delete() error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	name := w.Name()

	K8sClient.AppsV1().Deployments(WorkerNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	K8sClient.CoreV1().Services(WorkerNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	K8sClient.CoreV1().Services(IngressNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	w.deleteWorkerIngressRoute(ctx)
	return nil
}

func (w *Worker) deployWorkerDeployment(ctx context.Context) error {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name(),
			Namespace: WorkerNamespace,
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
					Containers: []corev1.Container{{
						Name:  w.Name(),
						Image: w.Image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: int32(w.Port),
						}},
					}},
				},
			},
		},
	}

	deploymentsClient := K8sClient.AppsV1().Deployments(WorkerNamespace)
	_, err := deploymentsClient.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{})
	} else if err == nil {
		_, err = deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{})
	}
	return err
}

func (w *Worker) deployWorkerService(ctx context.Context) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name(),
			Namespace: WorkerNamespace,
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

	servicesClient := K8sClient.CoreV1().Services(WorkerNamespace)
	_, err := servicesClient.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = servicesClient.Create(ctx, service, metav1.CreateOptions{})
	}
	return err
}

func (w *Worker) deployWorkerExternalService(ctx context.Context) error {
	externalName := fmt.Sprintf("%s.%s.svc.cluster.local", w.Name(), WorkerNamespace)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name(),
			Namespace: IngressNamespace,
			Labels:    w.Labels(),
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: externalName,
		},
	}

	servicesClient := K8sClient.CoreV1().Services(IngressNamespace)
	_, err := servicesClient.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = servicesClient.Create(ctx, service, metav1.CreateOptions{})
	}
	return err
}

func (w *Worker) deployWorkerIngressRoute(ctx context.Context) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	host := fmt.Sprintf("%s.worker.%s", w.Name(), Domain)

	ingressRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      w.Name(),
				"namespace": IngressNamespace,
				"labels": map[string]any{
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
								"name": w.Name(),
								"port": w.Port,
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

	client := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace)
	_, err := client.Get(ctx, w.Name(), metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, ingressRoute, metav1.CreateOptions{})
	} else if err == nil {
		_, err = client.Update(ctx, ingressRoute, metav1.UpdateOptions{})
	}
	return err
}

func (w *Worker) deleteWorkerIngressRoute(ctx context.Context) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}
	client := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace)
	return client.Delete(ctx, w.Name(), metav1.DeleteOptions{})
}

// ListWorkers lists all workers, optionally filtered by labels
func ListWorkers(workerId string, ownerId string) ([]Worker, error) {
	if K8sClient == nil {
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

	deployments, err := K8sClient.AppsV1().Deployments(WorkerNamespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}

	var workers []Worker
	for _, d := range deployments.Items {
		workers = append(workers, Worker{
			WorkerID: d.Labels["worker-id"],
			OwnerID:  d.Labels["owner-id"],
			Image:    d.Spec.Template.Spec.Containers[0].Image,
		})
	}
	return workers, nil
}
