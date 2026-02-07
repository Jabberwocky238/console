package handlers

import (
	"context"
	"log"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *WorkerHandler) process(t workerTask) {
	switch t.kind {
	case "deploy":
		h.deploy(t.versionID)
	case "sync_env":
		if err := syncEnvToConfigMap(t.workerID, t.userUID, t.data); err != nil {
			log.Printf("[worker] sync env configmap failed: %v", err)
			dblayer.UpdateWorkerStatus(t.workerID, "error")
		} else {
			dblayer.UpdateWorkerStatus(t.workerID, "active")
		}
	case "sync_secret":
		if err := syncSecretsToK8s(t.workerID, t.userUID, t.data); err != nil {
			log.Printf("[worker] sync k8s secret failed: %v", err)
			dblayer.UpdateWorkerStatus(t.workerID, "error")
		} else {
			dblayer.UpdateWorkerStatus(t.workerID, "active")
		}
	case "delete_cr":
		deleteWorkerCR(t.workerID, t.userUID)
	}
}

func (h *WorkerHandler) deploy(versionID int) {
	v, w, err := dblayer.GetDeployVersionWithWorker(versionID)
	if err != nil {
		log.Printf("[worker] get version %d failed: %v", versionID, err)
		dblayer.UpdateDeployVersionStatus(versionID, "error", err.Error())
		return
	}

	name := controller.WorkerName(w.WorkerID, w.UserUID)
	err = controller.CreateWorkerAppCR(
		k8s.DynamicClient, name,
		w.WorkerID, w.UserUID, v.Image, v.Port,
	)
	if err != nil {
		log.Printf("[worker] create CR for version %d failed: %v", versionID, err)
		dblayer.UpdateDeployVersionStatus(versionID, "error", err.Error())
		dblayer.UpdateWorkerStatus(v.WorkerID, "error")
		return
	}

	log.Printf("[worker] CR created for version %d", versionID)
	if err := dblayer.DeployVersionSuccess(versionID, v.WorkerID); err != nil {
		log.Printf("[worker] update deploy status failed: %v", err)
	}
}

func syncEnvToConfigMap(workerID, userUID string, envMap map[string]string) error {
	if k8s.K8sClient == nil {
		return nil
	}
	name := controller.WorkerName(workerID, userUID) + "-env"
	ctx := context.Background()
	client := k8s.K8sClient.CoreV1().ConfigMaps(k8s.WorkerNamespace)

	cm, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	cm.Data = envMap
	_, err = client.Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

func syncSecretsToK8s(workerID, userUID string, secretMap map[string]string) error {
	if k8s.K8sClient == nil {
		return nil
	}
	name := controller.WorkerName(workerID, userUID) + "-secret"
	ctx := context.Background()
	client := k8s.K8sClient.CoreV1().Secrets(k8s.WorkerNamespace)

	sec, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	data := make(map[string][]byte, len(secretMap))
	for k, v := range secretMap {
		data[k] = []byte(v)
	}
	sec.Data = data
	_, err = client.Update(ctx, sec, metav1.UpdateOptions{})
	return err
}

func deleteWorkerCR(workerID, userUID string) {
	name := controller.WorkerName(workerID, userUID)
	if err := controller.DeleteWorkerAppCR(k8s.DynamicClient, name); err != nil {
		log.Printf("[worker] failed to delete CR for %s: %v", workerID, err)
	}
}
