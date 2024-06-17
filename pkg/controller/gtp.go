package controller

import (
	"sync"

	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	"k8s.io/client-go/tools/cache"
)

type GlobalTrafficCache interface {
	GetFromIdentity(identity string, environment string) (*v1.GlobalTrafficPolicy, error)
	Put(gtp *v1.GlobalTrafficPolicy) error
	Delete(identity string, environment string) error
}

type GlobalTrafficController struct {
	CrdClient clientset.Interface
	Cache     *gtpCache
	informer  cache.SharedIndexInformer
}
type gtpItem struct {
	GlobalTrafficPolicy *v1.GlobalTrafficPolicy
	Status              string
}

type gtpCache struct {
	//map of gtps key=identity+env value is a map of gtps namespace -> map name -> gtp
	cache map[string]map[string]map[string]*gtpItem
	mutex *sync.Mutex
}
