package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/admiral-state-syncer/pkg/client"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/config"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/types"
	log "github.com/sirupsen/logrus"

	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/common"
	"github.com/istio-ecosystem/admiral/admiral/pkg/controller/util"
	"github.com/sirupsen/logrus"
	k8sAppsV1 "k8s.io/api/apps/v1"
	k8sAppsInformers "k8s.io/client-go/informers/apps/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"sync"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

type DeploymentItem struct {
	Deployment *k8sAppsV1.Deployment
	Status     string
}

type DeploymentClusterEntry struct {
	Identity    string
	Deployments map[string]*DeploymentItem
}

type DeploymentController struct {
	K8sClient      k8s.Interface
	Cache          *deploymentCache
	informer       cache.SharedIndexInformer
	labelSet       *common.LabelSet
	remoteRegistry *RemoteRegistry
	clusterID      string
}

type deploymentCache struct {
	//map of dependencies key=identity value array of onboarded identities
	cache map[string]*DeploymentClusterEntry
	mutex *sync.Mutex
}

func NewDeploymentController(
	stopCh <-chan struct{},
	config *rest.Config,
	resyncPeriod time.Duration,
	remoteRegistry *RemoteRegistry,
	clusterID string,
	clientLoader client.ClientLoader) (*DeploymentController, error) {
	var (
		err                  error
		deploymentController = DeploymentController{
			labelSet:       common.GetLabelSet(),
			Cache:          NewDeploymentCache(),
			remoteRegistry: remoteRegistry,
			clusterID:      clusterID,
		}
	)
	deploymentController.K8sClient, err = clientLoader.LoadKubeClientFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment controller k8s client: %v", err)
	}

	deploymentController.informer = k8sAppsInformers.NewDeploymentInformer(
		deploymentController.K8sClient,
		meta_v1.NamespaceAll,
		resyncPeriod,
		cache.Indexers{},
	)
	NewController(types.DeploymentControllerPrefix, config.Host, stopCh, &deploymentController, deploymentController.informer)
	return &deploymentController, nil
}

func (d *DeploymentController) Added(ctx context.Context, obj interface{}) error {
	return HandleAddUpdateDeployment(ctx, obj, d)
}

func (d *DeploymentController) Updated(ctx context.Context, obj interface{}, oldObj interface{}) error {
	return HandleAddUpdateDeployment(ctx, obj, d)
}

func (d *DeploymentController) GetProcessItemStatus(obj interface{}) (string, error) {
	deployment, ok := obj.(*k8sAppsV1.Deployment)
	if !ok {
		return types.NotProcessed, fmt.Errorf("type assertion failed, %v is not of type *v1.Deployment", obj)
	}
	return d.Cache.GetDeploymentProcessStatus(deployment), nil
}

func (d *DeploymentController) UpdateProcessItemStatus(obj interface{}, status string) error {
	deployment, ok := obj.(*k8sAppsV1.Deployment)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Deployment", obj)
	}
	return d.Cache.UpdateDeploymentProcessStatus(deployment, status)
}

func NewDeploymentCache() *deploymentCache {
	return &deploymentCache{
		cache: make(map[string]*DeploymentClusterEntry),
		mutex: &sync.Mutex{},
	}
}

func (p *deploymentCache) getKey(deployment *k8sAppsV1.Deployment) string {
	return common.GetDeploymentGlobalIdentifier(deployment)
}

func (p *deploymentCache) Get(key string, env string) *k8sAppsV1.Deployment {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	dce, ok := p.cache[key]
	if ok {
		dceEnv, ok := dce.Deployments[env]
		if ok {
			return dceEnv.Deployment
		}
	}

	return nil
}

func (d *deploymentCache) List() []k8sAppsV1.Deployment {
	var deploymentList []k8sAppsV1.Deployment
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for _, deploymentClusterEntry := range d.cache {
		for _, deploymentItem := range deploymentClusterEntry.Deployments {
			if deploymentItem != nil && deploymentItem.Deployment != nil {
				deploymentList = append(deploymentList, *deploymentItem.Deployment)
			}
		}
	}
	return deploymentList
}

func (p *deploymentCache) GetDeploymentProcessStatus(deployment *k8sAppsV1.Deployment) string {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnv(deployment)
	key := p.getKey(deployment)

	dce, ok := p.cache[key]
	if ok {
		dceEnv, ok := dce.Deployments[env]
		if ok {
			return dceEnv.Status
		}
	}
	return types.NotProcessed
}

func (p *deploymentCache) UpdateDeploymentProcessStatus(deployment *k8sAppsV1.Deployment, status string) error {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnv(deployment)
	key := p.getKey(deployment)

	dce, ok := p.cache[key]
	if ok {
		dceEnv, ok := dce.Deployments[env]
		if ok {
			dceEnv.Status = status
			p.cache[dce.Identity] = dce
			return nil
		} else {
			dce.Deployments[env] = &DeploymentItem{
				Status: status,
			}

			p.cache[dce.Identity] = dce
			return nil
		}
	}
	return fmt.Errorf(types.LogCacheFormat, "Update", "Deployment",
		deployment.Name, deployment.Namespace, "", "nothing to update, deployment not found in cache for env="+env)
}

func (p *deploymentCache) GetByIdentity(key string) map[string]*DeploymentItem {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	dce := p.cache[key]
	if dce != nil {
		return dce.Deployments
	}
	return nil
}

func (p *deploymentCache) UpdateDeploymentToClusterCache(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()

	env := common.GetEnv(deployment)
	dce := p.cache[key]
	if dce == nil {
		dce = &DeploymentClusterEntry{
			Identity:    key,
			Deployments: make(map[string]*DeploymentItem),
		}
	}

	dce.Deployments[env] = &DeploymentItem{
		Deployment: deployment,
		Status:     types.ProcessingInProgress,
	}
	p.cache[dce.Identity] = dce
}

func (p *deploymentCache) DeleteFromDeploymentClusterCache(key string, deployment *k8sAppsV1.Deployment) {
	defer p.mutex.Unlock()
	p.mutex.Lock()
	var (
		env = common.GetEnv(deployment)
		dce = p.cache[key]
	)

	if dce != nil {
		if dce.Deployments[env] != nil && dce.Deployments[env].Deployment != nil && deployment.Name == dce.Deployments[env].Deployment.Name {
			logrus.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Deployment",
				deployment.Name, deployment.Namespace, "", "ignoring deployment and deleting from cache")
			delete(dce.Deployments, env)
		} else {
			logrus.Warnf("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Get", "Deployment",
				deployment.Name, deployment.Namespace, "", "ignoring deployment delete as it doesn't match the one in cache")
		}
	} else {
		logrus.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", "Deployment",
			deployment.Name, deployment.Namespace, "", "nothing to delete, deployment not found in cache")
	}
}

func HandleAddUpdateDeployment(ctx context.Context, obj interface{}, d *DeploymentController) error {
	deployment, ok := obj.(*k8sAppsV1.Deployment)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Deployment", obj)
	}
	key := d.Cache.getKey(deployment)
	defer util.LogElapsedTime("HandleAddUpdateDeployment", key, deployment.Name+"_"+deployment.Namespace, "")()
	if len(key) > 0 {
		if !d.shouldIgnoreBasedOnLabels(ctx, deployment) {
			d.Cache.UpdateDeploymentToClusterCache(key, deployment)
			d.Cache.UpdateDeploymentToClusterCache(GetDeploymentOriginalIdentifier(deployment), deployment)
			return HandleEventForDeployment(ctx, types.Add, deployment, d.remoteRegistry, d.clusterID)
		} else {
			ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, deployment.Namespace, meta_v1.GetOptions{})
			if err != nil {
				logrus.Warnf("failed to get namespace object for deployment with namespace %v, err: %v", deployment.Namespace, err)
			} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || deployment.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
				logrus.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.DeploymentResourceType,
					deployment.Name, deployment.Namespace, "", "Value=true")
			}
			d.Cache.DeleteFromDeploymentClusterCache(key, deployment)
			d.Cache.DeleteFromDeploymentClusterCache(GetDeploymentOriginalIdentifier(deployment), deployment)
		}
	}
	return nil
}

func (d *DeploymentController) Deleted(ctx context.Context, obj interface{}) error {
	deployment, ok := obj.(*k8sAppsV1.Deployment)
	if !ok {
		return fmt.Errorf("type assertion failed, %v is not of type *v1.Deployment", obj)
	}
	if d.shouldIgnoreBasedOnLabels(ctx, deployment) {
		ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, deployment.Namespace, meta_v1.GetOptions{})
		if err != nil {
			logrus.Warnf("Failed to get namespace object for deployment with namespace %v, err: %v", deployment.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || deployment.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			logrus.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.DeploymentResourceType,
				deployment.Name, deployment.Namespace, "", "Value=true")
		}
		logrus.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "Delete", common.DeploymentResourceType,
			deployment.Name, deployment.Namespace, "", "ignoring deployment on basis of labels/annotation")
		return nil
	}
	key := d.Cache.getKey(deployment)
	err := HandleEventForDeployment(ctx, types.Delete, deployment, d.remoteRegistry, d.clusterID)
	if err == nil && len(key) > 0 {
		d.Cache.DeleteFromDeploymentClusterCache(key, deployment)
		d.Cache.DeleteFromDeploymentClusterCache(GetDeploymentOriginalIdentifier(deployment), deployment)
	}
	return err
}

func (d *DeploymentController) shouldIgnoreBasedOnLabels(ctx context.Context, deployment *k8sAppsV1.Deployment) bool {
	if deployment.Spec.Template.Labels[d.labelSet.AdmiralIgnoreLabel] == "true" { //if we should ignore, do that and who cares what else is there
		return true
	}

	if deployment.Spec.Template.Annotations[d.labelSet.DeploymentAnnotation] != "true" { //Not sidecar injected, we don't want to inject
		return true
	}

	if deployment.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}

	ns, err := d.K8sClient.CoreV1().Namespaces().Get(ctx, deployment.Namespace, meta_v1.GetOptions{})
	if err != nil {
		logrus.Warnf("Failed to get namespace object for deployment with namespace %v, err: %v", deployment.Namespace, err)
		return false
	}

	if ns.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
		return true
	}
	return false
}

func (d *DeploymentController) GetDeploymentBySelectorInNamespace(ctx context.Context, serviceSelector map[string]string, namespace string) []k8sAppsV1.Deployment {
	matchedDeployments, err := d.K8sClient.AppsV1().Deployments(namespace).List(ctx, meta_v1.ListOptions{})
	if err != nil {
		logrus.Errorf("Failed to list deployments in cluster, error: %v", err)
		return nil
	}

	if matchedDeployments.Items == nil {
		return []k8sAppsV1.Deployment{}
	}

	filteredDeployments := make([]k8sAppsV1.Deployment, 0)
	for _, deployment := range matchedDeployments.Items {
		if common.IsServiceMatch(serviceSelector, deployment.Spec.Selector) {
			filteredDeployments = append(filteredDeployments, deployment)
		}
	}
	return filteredDeployments
}

func (d *DeploymentController) LogValueOfAdmiralIoIgnore(obj interface{}) {
	deployment, ok := obj.(*k8sAppsV1.Deployment)
	if !ok {
		return
	}
	if d.K8sClient != nil {
		ns, err := d.K8sClient.CoreV1().Namespaces().Get(context.Background(), deployment.Namespace, meta_v1.GetOptions{})
		if err != nil {
			logrus.Warnf("Failed to get namespace object for deployment with namespace %v, err: %v", deployment.Namespace, err)
		} else if (ns != nil && ns.Annotations[common.AdmiralIgnoreAnnotation] == "true") || deployment.Annotations[common.AdmiralIgnoreAnnotation] == "true" {
			logrus.Infof("op=%s type=%v name=%v namespace=%s cluster=%s message=%s", "admiralIoIgnoreAnnotationCheck", common.DeploymentResourceType,
				deployment.Name, deployment.Namespace, "", "Value=true")
		}
	}
}

func (d *DeploymentController) Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error) {
	deployment, ok := obj.(*k8sAppsV1.Deployment)
	if ok && isRetry {
		return d.Cache.Get(common.GetDeploymentGlobalIdentifier(deployment), common.GetEnv(deployment)), nil
	}
	if ok && d.K8sClient != nil {
		return d.K8sClient.AppsV1().Deployments(deployment.Namespace).Get(ctx, deployment.Name, meta_v1.GetOptions{})
	}
	return nil, fmt.Errorf("kubernetes client is not initialized, txId=%s", ctx.Value("txId"))
}

// helper function to handle add and delete for DeploymentHandler
func HandleEventForDeployment(ctx context.Context, event types.EventType, obj *k8sAppsV1.Deployment,
	remoteRegistry *RemoteRegistry, clusterName string) error {

	log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, common.ReceivedStatus)
	globalIdentifier := common.GetDeploymentGlobalIdentifier(obj)
	log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "globalIdentifier is "+globalIdentifier)
	originalIdentifier := GetDeploymentOriginalIdentifier(obj)
	log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "originalIdentifier is "+originalIdentifier)

	if len(globalIdentifier) == 0 {
		log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "Skipped as '"+common.GetWorkloadIdentifier()+" was not found', namespace="+obj.Namespace)
		return nil
	}

	env := common.GetEnv(obj)

	ctx = context.WithValue(ctx, common.ClusterName, clusterName)
	ctx = context.WithValue(ctx, common.EventResourceType, common.Deployment)

	if remoteRegistry.AdmiralCache != nil {
		if remoteRegistry.AdmiralCache.IdentityClusterCache != nil {
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(globalIdentifier, clusterName, clusterName)
			remoteRegistry.AdmiralCache.IdentityClusterCache.Put(originalIdentifier, clusterName, clusterName)
			log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "for "+globalIdentifier+" got "+remoteRegistry.AdmiralCache.IdentityClusterCache.Get(globalIdentifier).Get(clusterName))
			log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "for "+originalIdentifier+" got "+remoteRegistry.AdmiralCache.IdentityClusterCache.Get(originalIdentifier).Get(clusterName))
		}
		if config.EnableSWAwareNSCaches() {
			if remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache != nil {
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(globalIdentifier, clusterName, obj.Namespace, obj.Namespace)
				remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Put(originalIdentifier, clusterName, obj.Namespace, obj.Namespace)
				log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "for "+globalIdentifier+" got "+remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Get(globalIdentifier).Get(clusterName).Get(obj.Namespace))
				log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "for "+originalIdentifier+" got "+remoteRegistry.AdmiralCache.IdentityClusterNamespaceCache.Get(originalIdentifier).Get(clusterName).Get(obj.Namespace))
			}
			if remoteRegistry.AdmiralCache.PartitionIdentityCache != nil && len(GetDeploymentIdentityPartition(obj)) > 0 {
				remoteRegistry.AdmiralCache.PartitionIdentityCache.Put(globalIdentifier, originalIdentifier)
				log.Infof(types.LogFormat, event, common.DeploymentResourceType, obj.Name, clusterName, "PartitionIdentityCachePut "+globalIdentifier+" for "+originalIdentifier)
			}
		}
	}

	// Use the same function as added deployment function to update and put new service entry in place to replace old one
	log.Infof("call modify se for env=%s", env)
	if config.EnableSWAwareNSCaches() && globalIdentifier != originalIdentifier {
		log.Infof("call modify se for env=%s", env)
	}
	return nil
}
