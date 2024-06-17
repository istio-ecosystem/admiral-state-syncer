package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	argo "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/config"

	argoprojv1alpha1 "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned/typed/rollouts/v1alpha1"
	argoinformers "github.com/argoproj/argo-rollouts/pkg/client/informers/externalversions"
	loader "github.com/istio-ecosystem/admiral-state-syncer/pkg/client"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/types"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	rolloutControllerPrefix = "rollouts-ctrl"
)

type RolloutsEntry struct {
	Identity string
	Rollout  *argo.Rollout
}

type RolloutItem struct {
	Rollout *argo.Rollout
	Status  string
}

type RolloutClusterEntry struct {
	Identity string
	Rollouts map[string]*RolloutItem
}

type RolloutController struct {
	K8sClient      kubernetes.Interface
	RolloutClient  argoprojv1alpha1.ArgoprojV1alpha1Interface
	informer       cache.SharedIndexInformer
	Cache          *rolloutCache
	labelSet       *config.LabelSet
	clusterID      string
	remoteRegistry *RemoteRegistry
}

type rolloutCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*RolloutClusterEntry
	mutex *sync.Mutex
}

func NewRolloutCache() *rolloutCache {
	return &rolloutCache{
		cache: make(map[string]*RolloutClusterEntry),
		mutex: &sync.Mutex{},
	}
}

func (p *rolloutCache) Put(rolloutEntry *RolloutClusterEntry) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	p.cache[rolloutEntry.Identity] = rolloutEntry
}

func (p *rolloutCache) getKey(rollout *argo.Rollout) string {
	return common.GetRolloutGlobalIdentifier(rollout)
}

func (p *rolloutCache) GetByIdentity(key string) map[string]*RolloutItem {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	rce := p.cache[key]
	if rce == nil {
		return nil
	} else {
		return rce.Rollouts
	}
}

func (p *rolloutCache) Get(key string, env string) *argo.Rollout {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	rce, ok := p.cache[key]
	if ok {
		rceEnv, ok := rce.Rollouts[env]
		if ok {
			return rceEnv.Rollout
		}
	}

	return nil
}

func (p *rolloutCache) List() []argo.Rollout {
	var rolloutList []argo.Rollout
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for _, rolloutClusterEntry := range p.cache {
		for _, rolloutItem := range rolloutClusterEntry.Rollouts {
			if rolloutItem != nil && rolloutItem.Rollout != nil {
				rolloutList = append(rolloutList, *rolloutItem.Rollout)
			}
		}
	}
	return rolloutList
}

func (p *rolloutCache) Delete(pod *RolloutClusterEntry) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	delete(p.cache, pod.Identity)
}

func (p *rolloutCache) UpdateRolloutToClusterCache(key string, rollout *argo.Rollout) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForRollout(rollout)

	rce := p.cache[key]

	if rce == nil {
		rce = &RolloutClusterEntry{
			Identity: key,
			Rollouts: make(map[string]*RolloutItem),
		}
	}
	rce.Rollouts[env] = &RolloutItem{
		Rollout: rollout,
		Status:  common.ProcessingInProgress,
	}

	p.cache[rce.Identity] = rce
}

func (p *rolloutCache) DeleteFromRolloutToClusterCache(key string, rollout *argo.Rollout) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForRollout(rollout)

	rce := p.cache[key]

	if rce != nil {
		delete(rce.Rollouts, env)
	}
}

func (p *rolloutCache) GetRolloutProcessStatus(rollout *argo.Rollout) string {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForRollout(rollout)
	key := p.getKey(rollout)

	rce, ok := p.cache[key]
	if ok {
		rceEnv, ok := rce.Rollouts[env]
		if ok {
			return rceEnv.Status
		}
	}

	return common.NotProcessed
}

func (p *rolloutCache) UpdateRolloutProcessStatus(rollout *argo.Rollout, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnvForRollout(rollout)
	key := p.getKey(rollout)

	rce, ok := p.cache[key]
	if ok {
		rceEnv, ok := rce.Rollouts[env]
		if ok {
			rceEnv.Status = status
			p.cache[rce.Identity] = rce
			return nil
		} else {
			rce.Rollouts[env] = &RolloutItem{
				Status: status,
			}

			p.cache[rce.Identity] = rce
			return nil
		}
	}

	return fmt.Errorf(types.LogCacheFormat, "Update", "Rollout",
		rollout.Name, rollout.Namespace, "", "nothing to update, rollout not found in cache")
}

func (d *RolloutController) shouldIgnoreBasedOnLabelsForRollout(ctx context.Context, rollout *argo.Rollout) bool {
	if rollout.Spec.Template.Labels[d.labelSet.AdmiralIgnoreLabel] == "true" { //if we should ignore, do that and who cares what else is there
		return true
	}

	if rollout.Spec.Template.Annotations[d.labelSet.DeploymentAnnotation] != "true" { //Not sidecar injected, we don't want to inject
		return true
	}

	if rollout.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, rollout.Namespace, meta_v1.GetOptions{})
	if err != nil {
		log.Warnf("Failed to get namespace object for rollout with namespace %v, err: %v", rollout.Namespace, err)
		return false
	}

	if ns.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}
	return false //labels are fine, we should not ignore
}

func NewRolloutsController(
	stopCh <-chan struct{},
	restConfig *rest.Config,
	resyncPeriod time.Duration,
	remoteRegistry *RemoteRegistry,
	clusterID string,
	clientLoader loader.ClientLoader) (*RolloutController, error) {
	var (
		err        error
		controller = RolloutController{
			clusterID:      clusterID,
			remoteRegistry: remoteRegistry,
			labelSet:       config.GetLabelSet(),
			Cache:          NewRolloutCache(),
		}
	)

	rolloutClient, err := clientLoader.LoadArgoClientFromConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollouts controller argo client: %v", err)
	}

	controller.K8sClient, err = clientLoader.LoadKubeClientFromConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollouts controller k8s client: %v", err)
	}

	controller.RolloutClient = rolloutClient.ArgoprojV1alpha1()

	argoRolloutsInformerFactory := argoinformers.NewSharedInformerFactoryWithOptions(
		rolloutClient,
		resyncPeriod,
		argoinformers.WithNamespace(meta_v1.NamespaceAll))
	//Initialize informer
	controller.informer = argoRolloutsInformerFactory.Argoproj().V1alpha1().Rollouts().Informer()

	NewController(rolloutControllerPrefix, restConfig.Host, stopCh, &controller, controller.informer)
	return &controller, nil
}

func (roc *RolloutController) Added(ctx context.Context, obj interface{}) error {
	return HandleAddUpdateRollout(ctx, obj, roc)
}

func (roc *RolloutController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	return HandleAddUpdateRollout(ctx, obj, roc)
}

func HandleAddUpdateRollout(ctx context.Context, obj interface{}, roc *RolloutController) error {
	rollout, ok := obj.(*argo.Rollout)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *argo.Rollout", obj)
	}
	key := roc.Cache.getKey(rollout)
	defer util.LogElapsedTime("HandleAddUpdateRollout", key, rollout.Name+"_"+rollout.Namespace, "")()
	if len(key) > 0 {
		if !roc.shouldIgnoreBasedOnLabelsForRollout(ctx, rollout) {
			roc.Cache.UpdateRolloutToClusterCache(key, rollout)
			roc.Cache.UpdateRolloutToClusterCache(GetRolloutOriginalIdentifier(rollout), rollout)
			return HandleEventForRollout(ctx, types.Add, rollout, roc.remoteRegistry, roc.clusterID)
		} else {
			ns, err := roc.K8sClient.CoreV1().Namespaces().Get(ctx, rollout.Namespace, meta_v1.GetOptions{})
			if err != nil {
				log.Warnf("Failed to get namespace object for rollout with namespace %v, err: %v", rollout.Namespace, err)
			} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || rollout.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
				log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.RolloutResourceType,
					rollout.Name, rollout.Namespace, "", "Value=true")
			}
			roc.Cache.DeleteFromRolloutToClusterCache(key, rollout)
			roc.Cache.DeleteFromRolloutToClusterCache(GetRolloutOriginalIdentifier(rollout), rollout)
			log.Debugf("ignoring rollout %v based on labels", rollout.Name)
		}
	}
	return nil
}

func (roc *RolloutController) Deleted(ctx context.Context, obj interface{}) error {
	rollout, ok := obj.(*argo.Rollout)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *argo.Rollout", obj)
	}
	if roc.shouldIgnoreBasedOnLabelsForRollout(ctx, rollout) {
		ns, err := roc.K8sClient.CoreV1().Namespaces().Get(ctx, rollout.Namespace, meta_v1.GetOptions{})
		if err != nil {
			log.Warnf("Failed to get namespace object for rollout with namespace %v, err: %v", rollout.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || rollout.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.RolloutResourceType,
				rollout.Name, rollout.Namespace, "", "Value=true")
		}
		log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", common.RolloutResourceType,
			rollout.Name, rollout.Namespace, "", "ignoring rollout on basis of labels/annotation")
		return nil
	}
	key := roc.Cache.getKey(rollout)
	err := HandleEventForRollout(ctx, types.Delete, rollout, roc.remoteRegistry, roc.clusterID)
	if err == nil && len(key) > 0 {
		roc.Cache.DeleteFromRolloutToClusterCache(key, rollout)
		roc.Cache.DeleteFromRolloutToClusterCache(GetRolloutOriginalIdentifier(rollout), rollout)
	}
	return err
}

func (d *RolloutController) GetRolloutBySelectorInNamespace(ctx context.Context, serviceSelector map[string]string, namespace string) []argo.Rollout {

	matchedRollouts, err := d.RolloutClient.Rollouts(namespace).List(ctx, meta_v1.ListOptions{})

	if err != nil {
		logrus.Errorf("Failed to list rollouts in cluster, error: %v", err)
		return nil
	}

	if matchedRollouts.Items == nil {
		return make([]argo.Rollout, 0)
	}

	filteredRollouts := make([]argo.Rollout, 0)

	for _, rollout := range matchedRollouts.Items {
		if common.IsServiceMatch(serviceSelector, rollout.Spec.Selector) {
			filteredRollouts = append(filteredRollouts, rollout)
		}
	}

	return filteredRollouts
}

func (d *RolloutController) GetProcessItemStatus(obj interface{}) (string, error) {
	rollout, ok := obj.(*argo.Rollout)
	if !ok {
		return common.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *argo.Rollout", obj)
	}
	return d.Cache.GetRolloutProcessStatus(rollout), nil
}

func (d *RolloutController) UpdateProcessItemStatus(obj interface{}, status string) error {
	rollout, ok := obj.(*argo.Rollout)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *argo.Rollout", obj)
	}
	return d.Cache.UpdateRolloutProcessStatus(rollout, status)
}

func (d *RolloutController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	rollout, ok := obj.(*argo.Rollout)
	if !ok {
		return
	}
	if d.K8sClient != nil {
		ns, err := d.K8sClient.CoreV1().Namespaces().Get(context.Background(), rollout.Namespace, meta_v1.GetOptions{})
		if err != nil {
			log.Warnf("Failed to get namespace object for rollout with namespace %v, err: %v", rollout.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || rollout.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			log.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.RolloutResourceType,
				rollout.Name, rollout.Namespace, "", "Value=true")
		}
	}
}

func (d *RolloutController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	rollout, ok := obj.(*argo.Rollout)
	if ok && isRetry {
		return d.Cache.Get(d.Cache.getKey(rollout), rollout.Namespace), nil
	}
	if ok && d.RolloutClient != nil {
		return d.RolloutClient.Rollouts(rollout.Namespace).Get(ctx, rollout.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("rollout client is not initialized, txId=%s", ctx.Value("txId"))
}

func GetRolloutOriginalIdentifier(rollout *argo.Rollout) string {
	identity := rollout.Spec.Template.Labels[config.GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = rollout.Spec.Template.Annotations[config.GetWorkloadIdentifier()]
	}
	return identity
}

// HandleEventForRollout helper function to handle add and delete for RolloutHandler
func HandleEventForRollout(ctx context.Context, event types.EventType, obj *argo.Rollout,
	remoteRegistry *RemoteRegistry, clusterName string) error {
	log.Infof(types.LogFormat, event, common.RolloutResourceType, obj.Name, clusterName, common.ReceivedStatus)
	globalIdentifier := GetRolloutGlobalIdentifier(obj)
	originalIdentifier := GetRolloutOriginalIdentifier(obj)
	if len(globalIdentifier) == 0 {
		log.Infof(types.LogFormat, event, common.RolloutResourceType, obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return nil
	}
	env := GetEnvForRollout(obj)

	ctx = context.WithValue(ctx, "clusterName", clusterName)
	ctx = context.WithValue(ctx, "eventResourceType", common.Rollout)

	if remoteRegistry.AdmiralCache != nil {
		if remoteRegistry.AdmiralCache.IdentityClusterCache != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(globalIdentifier, clusterName, clusterName)
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(originalIdentifier, clusterName, clusterName)
		}
		if config.EnableSWAwareNSCaches() {
			if remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache != nil {
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(globalIdentifier, clusterName, obj.Namespace, obj.Namespace)
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(originalIdentifier, clusterName, obj.Namespace, obj.Namespace)
			}
			if remoteRegistry.AdmiralCache.PartitionIdentityCache != nil && len(GetRolloutIdentityPartition(obj)) > 0 {
				remoteRegistry.AdmiralCache.PartitionIdentityCache.Put(globalIdentifier, originalIdentifier)
			}
		}
	}

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	log.Infof("calling modifySE for env=%s", env)
	if config.EnableSWAwareNSCaches() && globalIdentifier != originalIdentifier {
		log.Infof("calling modifySE for env=%s", env)
	}
	return nil
}

func GetEnvForRollout(rollout *argo.Rollout) string {
	var environment = rollout.Spec.Template.Annotations[config.GetEnvKey()]
	if len(environment) == 0 {
		environment = rollout.Spec.Template.Labels[config.GetEnvKey()]
	}
	if len(environment) == 0 {
		environment = rollout.Spec.Template.Labels[types.EnvLabel]
	}
	if len(environment) == 0 {
		splitNamespace := strings.Split(rollout.Namespace, "-")
		if len(splitNamespace) > 1 {
			environment = splitNamespace[len(splitNamespace)-1]
		}
		log.Warnf("Using deprecated approach to deduce env from namespace for rollout, name=%v in namespace=%v", rollout.Name, rollout.Namespace)
	}
	if len(environment) == 0 {
		environment = types.Default
	}
	return environment
}

func GetRolloutIdentityPartition(rollout *argo.Rollout) string {
	identityPartition := rollout.Spec.Template.Annotations[config.GetPartitionIdentifier()]
	if len(identityPartition) == 0 {
		//In case partition is accidentally applied as Label
		identityPartition = rollout.Spec.Template.Labels[config.GetPartitionIdentifier()]
	}
	return identityPartition
}

func GetRolloutGlobalIdentifier(rollout *argo.Rollout) string {
	identity := rollout.Spec.Template.Labels[config.GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = rollout.Spec.Template.Annotations[config.GetWorkloadIdentifier()]
	}
	if config.EnableSWAwareNSCaches() && len(identity) > 0 && len(GetRolloutIdentityPartition(rollout)) > 0 {
		identity = GetRolloutIdentityPartition(rollout) + types.Sep + strings.ToLower(identity)
	}
	return identity
}
