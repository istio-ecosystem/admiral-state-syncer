package controller

import (
	"context"
	"sync"

	k8sV1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ServiceHandler interface {
	Added(ctx context.Context, obj *k8sV1.Service) error
	Updated(ctx context.Context, obj *k8sV1.Service) error
	Deleted(ctx context.Context, obj *k8sV1.Service) error
}

type ServiceItem struct {
	Service *k8sV1.Service
	Status  string
}

type ServiceClusterEntry struct {
	Identity string
	Service  map[string]map[string]*ServiceItem //maps namespace to a map of service name:service object
}

type ServiceController struct {
	K8sClient      kubernetes.Interface
	ServiceHandler ServiceHandler
	Cache          *serviceCache
	informer       cache.SharedIndexInformer
}

type serviceCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*ServiceClusterEntry
	mutex *sync.Mutex
}
