package jobs

import (
	"log"

	"jabberwocky238/console/k8s"
)

// --- Auth Job types (implement k8s.Job) ---

type registerUserJob struct {
	UserUID string
}

func NewRegisterUserJob(userUID string) *registerUserJob {
	return &registerUserJob{UserUID: userUID}
}

func (j *registerUserJob) Type() string {
	return "auth.register_user"
}

func (j *registerUserJob) ID() string {
	return j.UserUID
}

func (j *registerUserJob) Do() error {
	if k8s.RDBManager != nil {
		if err := k8s.RDBManager.InitUserRDB(j.UserUID); err != nil {
			log.Printf("Warning: Failed to init RDB for user %s: %v", j.UserUID, err)
		} else {
			log.Printf("RDB initialized for user %s", j.UserUID)
		}
	} else {
		log.Printf("Warning: RDBManager not initialized, skip RDB init for user %s", j.UserUID)
	}
	return nil
}
