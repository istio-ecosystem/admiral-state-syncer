package controller

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NodeController struct {
	K8sClient kubernetes.Interface
	Locality  *Locality
	informer  cache.SharedIndexInformer
}
type Locality struct {
	Region string
}
