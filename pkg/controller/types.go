package controller

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/istio-ecosystem/admiral-state-syncer/pkg/client"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/types"
	v1 "github.com/istio-ecosystem/admiral/admiral/pkg/apis/admiral/v1"
	clientset "github.com/istio-ecosystem/admiral/admiral/pkg/client/clientset/versioned"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/admiral"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/istio"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/secret"
	"istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ServiceEntrySuspender interface {
	SuspendUpdate(identity string, environment string) bool
}

type IgnoredIdentityCache struct {
	RWLock                 *sync.RWMutex
	Enabled                bool                `json:"enabled"`
	All                    bool                `json:"all"`
	ClusterEnvironment     string              `json:"clusterEnvironment"`
	EnvironmentsByIdentity map[string][]string `json:"environmentsByIdentities"`
}

type RemoteController struct {
	ClusterID                 string
	ApiServer                 string
	StartTime                 time.Time
	GlobalTraffic             *GlobalTrafficController
	DeploymentController      *DeploymentController
	ServiceController         *ServiceController
	NodeController            *NodeController
	ServiceEntryController    *istio.ServiceEntryController
	DestinationRuleController *istio.DestinationRuleController
	VirtualServiceController  *istio.VirtualServiceController
	SidecarController         *istio.SidecarController
	RolloutController         *RolloutController
	// OutlierDetectionController       *OutlierDetectionController
	// ClientConnectionConfigController *ClientConnectionConfigController
	stop chan struct{}
}

type AdmiralCache struct {
	CnameClusterCache          *common.MapOfMaps
	CnameDependentClusterCache *common.MapOfMaps
	CnameIdentityCache         *sync.Map
	IdentityClusterCache       *common.MapOfMaps
	ClusterLocalityCache       *common.MapOfMaps
	IdentityDependencyCache    *common.MapOfMaps
	ConfigMapController        admiral.ConfigMapControllerInterface //todo this should be in the remotecontrollers map once we expand it to have one configmap per cluster
	GlobalTrafficCache         GlobalTrafficCache                   //The cache needs to live in the handler because it needs access to deployments
	// OutlierDetectionCache               OutlierDetectionCache
	// ClientConnectionConfigCache         ClientConnectionConfigCache
	DependencyNamespaceCache            *common.SidecarEgressMap
	SeClusterCache                      *common.MapOfMaps
	SourceToDestinations                *sourceToDestinations //This cache is to fetch list of all dependencies for a given source identity,
	TrafficConfigIgnoreAssets           []string
	GatewayAssets                       []string
	argoRolloutsEnabled                 bool
	DynamoDbEndpointUpdateCache         *sync.Map
	TrafficConfigWorkingScope           []*regexp.Regexp // regex of assets that are visible to Cartographer
	IdentitiesWithAdditionalEndpoints   *sync.Map
	IdentityClusterNamespaceCache       *types.MapOfMapOfMaps
	CnameDependentClusterNamespaceCache *types.MapOfMapOfMaps
	PartitionIdentityCache              *common.Map
}

type RemoteRegistry struct {
	sync.Mutex
	remoteControllers     map[string]*RemoteController
	SecretController      *secret.Controller
	secretClient          k8s.Interface
	ctx                   context.Context
	AdmiralCache          *AdmiralCache
	StartTime             time.Time
	ServiceEntrySuspender ServiceEntrySuspender
	DependencyController  *admiral.DependencyController
	ClientLoader          client.ClientLoader
}

type RoutingPolicyEntry struct {
	Identity      string
	RoutingPolicy *v1.RoutingPolicy
}

type RoutingPolicyClusterEntry struct {
	Identity        string
	RoutingPolicies map[string]*v1.RoutingPolicy
}

type RoutingPolicyController struct {
	K8sClient   kubernetes.Interface
	CrdClient   clientset.Interface
	IstioClient versioned.Interface
	informer    cache.SharedIndexInformer
}

type sourceToDestinations struct {
	sourceDestinations map[string][]string
	mutex              *sync.Mutex
}
