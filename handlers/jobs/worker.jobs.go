package jobs

import (
	"context"
	"fmt"
	"log"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// --- Worker Job types (implement k8s.Job) ---

type deployWorkerJob struct {
	WorkerID  string
	UserUID   string
	VersionID int
}

func NewDeployWorkerJob(workerID, userUID string, versionID int) *deployWorkerJob {
	return &deployWorkerJob{
		WorkerID:  workerID,
		UserUID:   userUID,
		VersionID: versionID,
	}
}

func (j *deployWorkerJob) Type() string {
	return "worker.deploy_worker"
}

func (j *deployWorkerJob) ID() string {
	return fmt.Sprintf("%s-%s-%d", j.WorkerID, j.UserUID, j.VersionID)
}

func (j *deployWorkerJob) Do() error {
	v, w, sk, err := dblayer.GetDeployVersionWithWorker(j.VersionID)
	if err != nil {
		dblayer.UpdateDeployVersionStatus(j.VersionID, "error", err.Error())
		return fmt.Errorf("get version %d: %w", j.VersionID, err)
	}

	name := controller.WorkerName(w.WID, w.UserUID)
	err = controller.CreateWorkerAppCR(
		k8s.DynamicClient, name,
		w.WID, w.UserUID, v.Image, sk, v.Port,
	)
	if err != nil {
		dblayer.UpdateDeployVersionStatus(j.VersionID, "error", err.Error())
		dblayer.UpdateWorkerStatus(w.WID, "error")
		return fmt.Errorf("create CR for version %d: %w", j.VersionID, err)
	}

	log.Printf("[worker] CR created for version %d", j.VersionID)
	if err := dblayer.DeployVersionSuccess(j.VersionID, w.ID); err != nil {
		log.Printf("[worker] update deploy status failed: %v", err)
	}
	return nil
}

type syncEnvJob struct {
	WorkerID string
	UserUID  string
	Data     map[string]string
}

func NewSyncEnvJob(workerID, userUID string, data map[string]string) *syncEnvJob {
	return &syncEnvJob{
		WorkerID: workerID,
		UserUID:  userUID,
		Data:     data,
	}
}

func (j *syncEnvJob) Type() string {
	return "worker.sync_env"
}

func (j *syncEnvJob) ID() string {
	return j.WorkerID
}

func (j *syncEnvJob) Do() error {
	if k8s.K8sClient == nil {
		return nil
	}
	name := controller.WorkerName(j.WorkerID, j.UserUID) + "-env"
	ctx := context.Background()
	client := k8s.K8sClient.CoreV1().ConfigMaps(k8s.WorkerNamespace)

	cm, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	cm.Data = j.Data
	if _, err = client.Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		dblayer.UpdateWorkerStatus(j.WorkerID, "error")
		return fmt.Errorf("sync env configmap: %w", err)
	}
	dblayer.UpdateWorkerStatus(j.WorkerID, "active")
	return nil
}

type syncSecretJob struct {
	WorkerID string
	UserUID  string
	Data     map[string]string
}

func NewSyncSecretJob(workerID, userUID string, data map[string]string) *syncSecretJob {
	return &syncSecretJob{
		WorkerID: workerID,
		UserUID:  userUID,
		Data:     data,
	}
}

func (j *syncSecretJob) Type() string {
	return "worker.sync_secret"
}

func (j *syncSecretJob) ID() string {
	return j.WorkerID
}

func (j *syncSecretJob) Do() error {
	if k8s.K8sClient == nil {
		return nil
	}
	name := controller.WorkerName(j.WorkerID, j.UserUID) + "-secret"
	ctx := context.Background()
	client := k8s.K8sClient.CoreV1().Secrets(k8s.WorkerNamespace)

	sec, err := client.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	data := make(map[string][]byte, len(j.Data))
	for k, v := range j.Data {
		data[k] = []byte(v)
	}
	sec.Data = data
	if _, err = client.Update(ctx, sec, metav1.UpdateOptions{}); err != nil {
		dblayer.UpdateWorkerStatus(j.WorkerID, "error")
		return fmt.Errorf("sync secret: %w", err)
	}
	dblayer.UpdateWorkerStatus(j.WorkerID, "active")
	return nil
}

type deleteWorkerCRJob struct {
	WorkerID string
	UserUID  string
}

func NewDeleteWorkerCRJob(workerID, userUID string) *deleteWorkerCRJob {
	return &deleteWorkerCRJob{
		WorkerID: workerID,
		UserUID:  userUID,
	}
}

func (j *deleteWorkerCRJob) Type() string {
	return "worker.delete_worker_cr"
}

func (j *deleteWorkerCRJob) ID() string {
	return j.WorkerID
}

func (j *deleteWorkerCRJob) Do() error {
	name := controller.WorkerName(j.WorkerID, j.UserUID)
	return controller.DeleteWorkerAppCR(k8s.DynamicClient, name)
}
