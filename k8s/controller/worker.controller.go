package controller

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"jabberwocky238/console/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
)

type WorkerController struct {
	ctrl    *Controller
	crCache cache.Store
}

// --- CR event handlers ---

func (wc *WorkerController) onAdd(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	log.Printf("[controller] WorkerApp added: %s", u.GetName())
	wc.reconcile(u)
}

func (wc *WorkerController) onUpdate(oldObj, newObj interface{}) {
	oldU, ok1 := oldObj.(*unstructured.Unstructured)
	newU, ok2 := newObj.(*unstructured.Unstructured)
	if !ok1 || !ok2 {
		return
	}
	if oldU.GetGeneration() == newU.GetGeneration() {
		return // status-only update, skip reconcile
	}
	log.Printf("[controller] WorkerApp updated: %s", newU.GetName())
	wc.reconcile(newU)
}

func (wc *WorkerController) onDelete(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	log.Printf("[controller] WorkerApp deleted: %s", u.GetName())

	w := workerFromUnstructured(u)
	if w == nil {
		return
	}
	w.DeleteAll(context.Background())
}

// --- Sub-resource delete handler ---

func (wc *WorkerController) onSubResourceDelete(obj interface{}) {
	var appName string
	switch o := obj.(type) {
	case metav1.Object:
		appName = o.GetLabels()["app"]
	case *unstructured.Unstructured:
		appName = o.GetLabels()["app"]
	default:
		return
	}
	if appName == "" {
		return
	}

	key := k8s.WorkerNamespace + "/" + appName
	item, exists, err := wc.crCache.GetByKey(key)
	if err != nil || !exists {
		return
	}
	u, ok := item.(*unstructured.Unstructured)
	if !ok {
		return
	}
	log.Printf("[controller] sub-resource deleted for %s, re-reconciling", appName)
	wc.reconcile(u)
}

// --- ConfigMap / Secret update handler ---

func (wc *WorkerController) onConfigUpdate(oldObj, newObj interface{}) {
	old, ok1 := oldObj.(metav1.Object)
	cur, ok2 := newObj.(metav1.Object)
	if !ok1 || !ok2 {
		return
	}
	if old.GetResourceVersion() == cur.GetResourceVersion() {
		return
	}
	appName := cur.GetLabels()["app"]
	if appName == "" {
		return
	}
	log.Printf("[controller] config/secret updated for %s, restarting deployment", appName)
	wc.restartDeployment(appName)
}

func (wc *WorkerController) restartDeployment(name string) {
	if k8s.K8sClient == nil {
		return
	}
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"console.app238.com/restartedAt":"%s"}}}}}`,
		strconv.FormatInt(time.Now().Unix(), 10),
	)
	_, err := k8s.K8sClient.AppsV1().Deployments(k8s.WorkerNamespace).Patch(
		context.Background(), name, types.StrategicMergePatchType,
		[]byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		log.Printf("[controller] restart deployment %s failed: %v", name, err)
	}
}

// --- Reconcile ---

func (wc *WorkerController) reconcile(u *unstructured.Unstructured) {
	w := workerFromUnstructured(u)
	if w == nil {
		return
	}

	ctx := context.Background()
	wc.ctrl.updateStatus(u, WorkerAppGVR, "Deploying", "")

	if err := w.EnsureConfigMap(ctx); err != nil {
		log.Printf("[controller] ensure configmap for %s failed: %v", u.GetName(), err)
		wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
		return
	}
	if err := w.EnsureSecret(ctx); err != nil {
		log.Printf("[controller] ensure secret for %s failed: %v", u.GetName(), err)
		wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
		return
	}
	if err := w.EnsureDeployment(ctx); err != nil {
		log.Printf("[controller] ensure deployment for %s failed: %v", u.GetName(), err)
		wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
		return
	}
	if err := w.EnsureService(ctx); err != nil {
		log.Printf("[controller] ensure service for %s failed: %v", u.GetName(), err)
		wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
		return
	}
	if err := w.EnsureIngressRoute(ctx); err != nil {
		log.Printf("[controller] ensure ingress route for %s failed: %v", u.GetName(), err)
		wc.ctrl.updateStatus(u, WorkerAppGVR, "Failed", err.Error())
		return
	}

	log.Printf("[controller] reconcile %s success", u.GetName())
	wc.ctrl.updateStatus(u, WorkerAppGVR, "Running", "")
}

// --- Helpers ---

func workerFromUnstructured(u *unstructured.Unstructured) *WorkerAppSpec {
	spec, _ := u.Object["spec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	port, _ := spec["port"].(int64)
	return &WorkerAppSpec{
		WorkerID: fmt.Sprintf("%v", spec["workerID"]),
		OwnerID:  fmt.Sprintf("%v", spec["ownerID"]),
		OwnerSK:  fmt.Sprintf("%v", spec["ownerSK"]),
		Image:    fmt.Sprintf("%v", spec["image"]),
		Port:     int(port),
	}
}

// --- CR CRUD (used by handlers) ---

func CreateWorkerAppCR(
	client dynamic.Interface,
	name, workerID, ownerID, image string, ownerSK string,
	port int,
) error {
	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": Group + "/" + Version,
			"kind":       WorkerKind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": k8s.WorkerNamespace,
			},
			"spec": map[string]interface{}{
				"workerID": workerID,
				"ownerID":  ownerID,
				"ownerSK":  ownerSK,
				"image":    image,
				"port":     int64(port),
			},
		},
	}

	ctx := context.Background()
	res := client.Resource(WorkerAppGVR).Namespace(k8s.WorkerNamespace)

	existing, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		_, err = res.Create(ctx, cr, metav1.CreateOptions{})
		return err
	}

	cr.SetResourceVersion(existing.GetResourceVersion())
	_, err = res.Update(ctx, cr, metav1.UpdateOptions{})
	return err
}

func DeleteWorkerAppCR(client dynamic.Interface, name string) error {
	return client.Resource(WorkerAppGVR).
		Namespace(k8s.WorkerNamespace).
		Delete(context.Background(), name, metav1.DeleteOptions{})
}
