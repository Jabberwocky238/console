package controller

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Group              = "console.app238.com"
	Version            = "v1"
	WorkerResource     = "workerapps"
	WorkerKind         = "WorkerApp"
	CombinatorResource = "combinatorapps"
	CombinatorKind     = "CombinatorApp"
)

var WorkerAppGVR = schema.GroupVersionResource{
	Group:    Group,
	Version:  Version,
	Resource: WorkerResource,
}

type WorkerAppSpec struct {
	WorkerID    string `json:"workerID"`
	OwnerID     string `json:"ownerID"`
	OwnerSK     string `json:"ownerSK"`
	Image       string `json:"image"`
	Port        int    `json:"port"`
	AssignedCPU    string `json:"assignedCPU"`    // e.g. "1"
	AssignedMemory string `json:"assignedMemory"` // e.g. "500Mi"
	AssignedDisk   string `json:"assignedDisk"`   // e.g. "2Gi"
	MaxReplicas int    `json:"maxReplicas"` // e.g. 3
	MainRegion  string `json:"mainRegion"`  // e.g. "us-east-1"
}

type WorkerAppStatus struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
}
