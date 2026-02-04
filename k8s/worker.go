package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Worker represents a worker deployment
type Worker struct {
	WorkerID string
	OwnerID  string
	Image    string
	Port     int
}

// Name returns the worker's resource name
func (w *Worker) Name() string {
	return fmt.Sprintf("%s-%s", w.WorkerID, w.OwnerID)
}

// DeployWorker deploys a worker to K8s
func DeployWorker(worker *Worker) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	name := worker.Name()

	if err := deployWorkerDeployment(ctx, worker, name); err != nil {
		return fmt.Errorf("deploy deployment failed: %w", err)
	}
	if err := deployWorkerService(ctx, worker, name); err != nil {
		return fmt.Errorf("deploy service failed: %w", err)
	}
	if err := deployWorkerExternalService(ctx, worker, name); err != nil {
		return fmt.Errorf("deploy external service failed: %w", err)
	}
	if err := deployWorkerIngressRoute(ctx, worker, name); err != nil {
		return fmt.Errorf("deploy ingress route failed: %w", err)
	}
	return nil
}

// DeleteWorker deletes a worker from K8s
func DeleteWorker(worker *Worker) error {
	if K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	name := worker.Name()

	K8sClient.AppsV1().Deployments(WorkerNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	K8sClient.CoreV1().Services(WorkerNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	K8sClient.CoreV1().Services(IngressNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	deleteWorkerIngressRoute(ctx, name)
	return nil
}

func deployWorkerDeployment(ctx context.Context, worker *Worker, name string) error {
	replicas := int32(1)
	labels := map[string]string{
		"app":       name,
		"worker-id": worker.WorkerID,
		"owner-id":  worker.OwnerID,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: WorkerNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  name,
						Image: worker.Image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: int32(worker.Port),
						}},
					}},
				},
			},
		},
	}

	deploymentsClient := K8sClient.AppsV1().Deployments(WorkerNamespace)
	_, err := deploymentsClient.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{})
	} else if err == nil {
		_, err = deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{})
	}
	return err
}

func deployWorkerService(ctx context.Context, worker *Worker, name string) error {
	labels := map[string]string{
		"app":       name,
		"worker-id": worker.WorkerID,
		"owner-id":  worker.OwnerID,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: WorkerNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Port:     int32(worker.Port),
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	servicesClient := K8sClient.CoreV1().Services(WorkerNamespace)
	_, err := servicesClient.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = servicesClient.Create(ctx, service, metav1.CreateOptions{})
	}
	return err
}

func deployWorkerExternalService(ctx context.Context, worker *Worker, name string) error {
	labels := map[string]string{
		"app":       name,
		"worker-id": worker.WorkerID,
		"owner-id":  worker.OwnerID,
	}

	externalName := fmt.Sprintf("%s.%s.svc.cluster.local", name, WorkerNamespace)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: IngressNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: externalName,
		},
	}

	servicesClient := K8sClient.CoreV1().Services(IngressNamespace)
	_, err := servicesClient.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = servicesClient.Create(ctx, service, metav1.CreateOptions{})
	}
	return err
}

func deployWorkerIngressRoute(ctx context.Context, worker *Worker, name string) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}

	host := fmt.Sprintf("%s.worker.%s", name, Domain)

	ingressRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      name,
				"namespace": IngressNamespace,
				"labels": map[string]any{
					"worker-id": worker.WorkerID,
					"owner-id":  worker.OwnerID,
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
								"name": name,
								"port": worker.Port,
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
	_, err := client.Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = client.Create(ctx, ingressRoute, metav1.CreateOptions{})
	} else if err == nil {
		_, err = client.Update(ctx, ingressRoute, metav1.UpdateOptions{})
	}
	return err
}

func deleteWorkerIngressRoute(ctx context.Context, name string) error {
	if DynamicClient == nil {
		return fmt.Errorf("dynamic client not initialized")
	}
	client := DynamicClient.Resource(ingressRouteGVR).Namespace(IngressNamespace)
	return client.Delete(ctx, name, metav1.DeleteOptions{})
}
