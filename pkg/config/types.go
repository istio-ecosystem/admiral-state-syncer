package config

import (
	"sync"

	"github.com/matryer/resync"
)

type Params struct {
	LabelSet LabelSet
}

type LabelSet struct {
	DeploymentAnnotation                string
	SubsetLabel                         string
	NamespaceSidecarInjectionLabel      string
	NamespaceSidecarInjectionLabelValue string
	AdmiralIgnoreLabel                  string
	PriorityKey                         string
	WorkloadIdentityKey                 string //Should always be used for both label and annotation (using label as the primary, and falling back to annotation if the label is not found)
}

type paramsWrapper struct {
	params Params
	sync.RWMutex
	resync.Once
}
