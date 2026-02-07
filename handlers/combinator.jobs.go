package handlers

import (
	"fmt"
	"log"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"
)

// --- CreateRDBJob ---

type CreateRDBJob struct {
	RecordID   string // combinator_resources.id
	UserUID    string
	Name       string
	ResourceID string
}

func NewCreateRDBJob(recordID, userUID, name, resourceID string) *CreateRDBJob {
	return &CreateRDBJob{
		RecordID:   recordID,
		UserUID:    userUID,
		Name:       name,
		ResourceID: resourceID,
	}
}

func (j *CreateRDBJob) Type() string { return "combinator.create_rdb" }
func (j *CreateRDBJob) ID() string   { return j.RecordID }

func (j *CreateRDBJob) Do() error {
	if k8s.RDBManager == nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", "cockroachdb not available")
		return fmt.Errorf("cockroachdb not available")
	}
	if err := k8s.RDBManager.CreateSchema(j.UserUID, j.ResourceID); err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("create schema: %w", err)
	}

	cfg, err := controller.GetCombinatorConfig(k8s.DynamicClient, j.UserUID)
	if err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("get combinator config: %w", err)
	}

	cfg.RDBs = append(cfg.RDBs, controller.RDBItem{
		ID:   j.ResourceID,
		Name: j.Name,
		URL:  k8s.RDBManager.DSNWithSchema(j.UserUID, j.ResourceID),
	})

	if err := controller.UpdateCombinatorConfig(k8s.DynamicClient, j.UserUID, cfg); err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("update CR config: %w", err)
	}

	dblayer.UpdateCombinatorResourceStatus(j.RecordID, "active", "")
	log.Printf("[combinator] RDB %s created for user %s", j.ResourceID, j.UserUID)
	return nil
}

// --- DeleteRDBJob ---

type DeleteRDBJob struct {
	UserUID    string
	ResourceID string
}

func NewDeleteRDBJob(userUID, resourceID string) *DeleteRDBJob {
	return &DeleteRDBJob{UserUID: userUID, ResourceID: resourceID}
}

func (j *DeleteRDBJob) Type() string { return "combinator.delete_rdb" }
func (j *DeleteRDBJob) ID() string   { return j.ResourceID }

func (j *DeleteRDBJob) Do() error {
	if k8s.RDBManager != nil {
		if err := k8s.RDBManager.DeleteSchema(j.UserUID, j.ResourceID); err != nil {
			log.Printf("[combinator] delete schema %s failed: %v", j.ResourceID, err)
		}
	}

	cfg, err := controller.GetCombinatorConfig(k8s.DynamicClient, j.UserUID)
	if err != nil {
		return fmt.Errorf("get combinator config: %w", err)
	}

	for i, rdb := range cfg.RDBs {
		if rdb.ID == j.ResourceID {
			cfg.RDBs = append(cfg.RDBs[:i], cfg.RDBs[i+1:]...)
			break
		}
	}

	if err := controller.UpdateCombinatorConfig(k8s.DynamicClient, j.UserUID, cfg); err != nil {
		return fmt.Errorf("update CR config: %w", err)
	}

	log.Printf("[combinator] RDB %s deleted for user %s", j.ResourceID, j.UserUID)
	return nil
}

// --- CreateKVJob ---

type CreateKVJob struct {
	RecordID   string
	UserUID    string
	ResourceID string
	KVType     string
	KVURL      string
}

func NewCreateKVJob(recordID, userUID, resourceID, kvType, kvURL string) *CreateKVJob {
	return &CreateKVJob{
		RecordID:   recordID,
		UserUID:    userUID,
		ResourceID: resourceID,
		KVType:     kvType,
		KVURL:      kvURL,
	}
}

func (j *CreateKVJob) Type() string { return "combinator.create_kv" }
func (j *CreateKVJob) ID() string   { return j.RecordID }

func (j *CreateKVJob) Do() error {
	cfg, err := controller.GetCombinatorConfig(k8s.DynamicClient, j.UserUID)
	if err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("get combinator config: %w", err)
	}

	cfg.KVs = append(cfg.KVs, controller.KVItem{
		ID:   j.ResourceID,
		Type: j.KVType,
		URL:  j.KVURL,
	})

	if err := controller.UpdateCombinatorConfig(k8s.DynamicClient, j.UserUID, cfg); err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("update CR config: %w", err)
	}

	dblayer.UpdateCombinatorResourceStatus(j.RecordID, "active", "")
	log.Printf("[combinator] KV %s created for user %s", j.ResourceID, j.UserUID)
	return nil
}

// --- DeleteKVJob ---

type DeleteKVJob struct {
	UserUID    string
	ResourceID string
}

func NewDeleteKVJob(userUID, resourceID string) *DeleteKVJob {
	return &DeleteKVJob{UserUID: userUID, ResourceID: resourceID}
}

func (j *DeleteKVJob) Type() string { return "combinator.delete_kv" }
func (j *DeleteKVJob) ID() string   { return j.ResourceID }

func (j *DeleteKVJob) Do() error {
	cfg, err := controller.GetCombinatorConfig(k8s.DynamicClient, j.UserUID)
	if err != nil {
		return fmt.Errorf("get combinator config: %w", err)
	}

	for i, kv := range cfg.KVs {
		if kv.ID == j.ResourceID {
			cfg.KVs = append(cfg.KVs[:i], cfg.KVs[i+1:]...)
			break
		}
	}

	if err := controller.UpdateCombinatorConfig(k8s.DynamicClient, j.UserUID, cfg); err != nil {
		return fmt.Errorf("update CR config: %w", err)
	}

	log.Printf("[combinator] KV %s deleted for user %s", j.ResourceID, j.UserUID)
	return nil
}
