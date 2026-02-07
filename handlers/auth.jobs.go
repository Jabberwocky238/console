package handlers

import (
	"log"

	"jabberwocky238/console/k8s"
	"jabberwocky238/console/k8s/controller"
)

// --- Auth Job types (implement k8s.Job) ---

type RegisterUserJob struct {
	UserUID string
}

func (j *RegisterUserJob) Type() string {
	return "auth.register_user"
}

func (j *RegisterUserJob) ID() string {
	return j.UserUID
}

func (j *RegisterUserJob) Do() error {
	if k8s.RDBManager != nil {
		if err := k8s.RDBManager.InitUserRDB(j.UserUID); err != nil {
			log.Printf("Warning: Failed to init RDB for user %s: %v", j.UserUID, err)
		}
	} else {
		log.Printf("Warning: RDBManager not initialized, skip RDB init for user %s", j.UserUID)
	}

	if k8s.DynamicClient == nil {
		log.Printf("Warning: dynamic client not initialized, skip CR creation for user %s", j.UserUID)
		return nil
	}
	config := controller.EmptyCombinatorConfig()
	if err := controller.CreateCombinatorAppCR(k8s.DynamicClient, j.UserUID, config); err != nil {
		log.Printf("Warning: Failed to create CombinatorApp CR for user %s: %v", j.UserUID, err)
	}
	return nil
}
