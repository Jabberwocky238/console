package handlers

import (
	"fmt"
	"log"

	"jabberwocky238/console/dblayer"
	"jabberwocky238/console/k8s"
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
	if err := k8s.RDBManager.InitUserRDB(j.UserUID); err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("init user rdb: %w", err)
	}
	if err := k8s.RDBManager.CreateSchema(j.UserUID, j.ResourceID); err != nil {
		dblayer.UpdateCombinatorResourceStatus(j.RecordID, "error", err.Error())
		return fmt.Errorf("create schema: %w", err)
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

	log.Printf("[combinator] RDB %s deleted for user %s", j.ResourceID, j.UserUID)
	return nil
}

// --- CreateKVJob ---

type CreateKVJob struct {
	RecordID   string // combinator_resources.id
	UserUID    string
	ResourceID string
}

func NewCreateKVJob(recordID, userUID, resourceID string) *CreateKVJob {
	return &CreateKVJob{
		RecordID:   recordID,
		UserUID:    userUID,
		ResourceID: resourceID,
	}
}

func (j *CreateKVJob) Type() string { return "combinator.create_kv" }
func (j *CreateKVJob) ID() string   { return j.RecordID }

func (j *CreateKVJob) Do() error {
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

	log.Printf("[combinator] KV %s deleted for user %s", j.ResourceID, j.UserUID)
	return nil
}
