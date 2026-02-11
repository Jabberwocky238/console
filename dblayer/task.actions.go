package dblayer

import (
	"time"
)

type TaskStatusType string

const (
	TaskStatusProcessing = "processing"
	TaskStatusFinished   = "finished"
)

type ConsoleTask struct {
	ID                 int            `json:"id"`
	TaskType           string         `json:"task_type"`
	TaskStatus         TaskStatusType `json:"task_status"`
	TaskDetailedStatus string         `json:"task_detailed_status"`
	TaskInfo           string         `json:"task_info"`
	CreatedAt          time.Time      `json:"created_at"`
}

// CreateTask creates a new console task
func CreateTask(taskType, detailedStatus, taskInfo string, status TaskStatusType) (*ConsoleTask, error) {
	query := `
		INSERT INTO console_tasks (task_type, task_status, task_detailed_status, task_info)
		VALUES ($1, $2, $3, $4)
		RETURNING id, task_type, task_status, task_detailed_status, task_info, created_at
	`

	task := &ConsoleTask{}
	err := DB.QueryRow(query, taskType, status, detailedStatus, taskInfo).Scan(
		&task.ID,
		&task.TaskType,
		&task.TaskStatus,
		&task.TaskDetailedStatus,
		&task.TaskInfo,
		&task.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return task, nil
}

// UpdateTaskStatus updates the status and detailed status of a task
func UpdateTaskStatus(taskID int, status TaskStatusType, detailedStatus string) error {
	query := `
		UPDATE console_tasks
		SET task_status = $1, task_detailed_status = $2
		WHERE id = $3
	`

	_, err := DB.Exec(query, status, detailedStatus, taskID)
	return err
}

// GetAllPendingTasks retrieves all tasks with pending status
func GetAllPendingTasks() ([]ConsoleTask, error) {
	query := `
		SELECT id, task_type, task_status, task_detailed_status, task_info, created_at
		FROM console_tasks
		WHERE task_status = 'pending'
		ORDER BY created_at ASC
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []ConsoleTask
	for rows.Next() {
		var task ConsoleTask
		err := rows.Scan(
			&task.ID,
			&task.TaskType,
			&task.TaskStatus,
			&task.TaskDetailedStatus,
			&task.TaskInfo,
			&task.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}
