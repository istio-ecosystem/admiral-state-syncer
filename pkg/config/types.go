package config

import (
	"sync"

	"github.com/matryer/resync"
)

type Params struct {
	LabelSet              LabelSet
	EnableSWAwareNSCaches bool
}

type LabelSet struct {
	DeploymentAnnotation                string
	SubsetLabel                         string
	NamespaceSidecarInjectionLabel      string
	NamespaceSidecarInjectionLabelValue string
	AdmiralIgnoreLabel                  string
	PriorityKey                         string
	WorkloadIdentityKey                 string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
	IdentityPartitionKey                string //Label used for partitioning assets with same identity into groups
}

type paramsWrapper struct {
	params Params
	sync.RWMutex
	resync.Once
}
