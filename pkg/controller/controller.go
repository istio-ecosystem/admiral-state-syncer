package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/istio-ecosystem/admiral-state-syncer/pkg/monitoring"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/types"
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/util"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	maxRetries = 2
	// operations
	operationInformerEvents = "informerEvents"
	// tasks
	taskAddEventToQueue           = "addEventToQueue"
	taskGetEventFromQueue         = "getEventFromQueue"
	taskSendEventToDelegator      = "sendEventToDelegator"
	taskReceiveEventFromDelegator = "receivedEventFromDelegator"
	taskRequeueAttempt            = "requeueAttempt"
	taskGivingUpEvent             = "givingUpEvent"
	taskRequeueEvent              = "requeueEvent"
)

var (
	// Log Formats
	ControllerLogFormat = monitoring.NewTaskFormatter("len=%v")
	LogQueueFormat      = "op=" +
		operationInformerEvents +
		" " +
		monitoring.NewTaskFormatter("controller=%v cluster=%v len=%v")
)

// Delegator interface contains the methods that are required
type Delegator interface {
	Added(context.Context, interface{}) error
	Updated(context.Context, interface{}, interface{}) error
	Deleted(context.Context, interface{}) error
	UpdateProcessItemStatus(interface{}, string) error
	GetProcessItemStatus(interface{}) (string, error)
	LogValueOfAdmiralIoIgnore(interface{})
	Get(ctx context.Context, isRetry bool, obj interface{}) (interface{}, error)
}

type EventType string

const (
	Add    EventType = "Add"
	Update EventType = "Update"
	Delete EventType = "Delete"
)

type InformerCacheObj struct {
	key       string
	eventType EventType
	obj       interface{}
	oldObj    interface{}
	txId      string
	ctxLogger *logrus.Entry
}

type Controller struct {
	name      string
	cluster   string
	delegator Delegator
	queue     workqueue.RateLimitingInterface
	informer  cache.SharedIndexInformer
}

func NewController(name, clusterEndpoint string, stopCh <-chan struct{}, delegator Delegator, informer cache.SharedIndexInformer) Controller {
	controller := Controller{
		name:      name,
		cluster:   clusterEndpoint,
		informer:  informer,
		delegator: delegator,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
	controller.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			var (
				txId                    = uuid.NewString()
				metaName, metaNamespace string
			)
			meta, ok := obj.(metav1.Object)
			if ok && meta != nil && meta.GetResourceVersion() != "" {
				txId = monitoring.GenerateTxId(meta, controller.name, txId)
				metaName = meta.GetName()
				metaNamespace = meta.GetNamespace()
			}
			ctxLogger := logrus.WithFields(logrus.Fields{
				"op":         operationInformerEvents,
				"name":       metaName,
				"namespace":  metaNamespace,
				"controller": controller.name,
				"cluster":    controller.cluster,
				"txId":       txId,
			})
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				ctxLogger.Infof(ControllerLogFormat, taskAddEventToQueue, controller.queue.Len(), Add+" Event")
				controller.queue.Add(InformerCacheObj{
					key:       key,
					eventType: Add,
					obj:       obj,
					txId:      txId,
					ctxLogger: ctxLogger,
				})
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			var (
				ctx                     = context.Background()
				txId                    = uuid.NewString()
				metaName, metaNamespace string
			)
			meta, ok := newObj.(metav1.Object)
			if ok && meta != nil && meta.GetResourceVersion() != "" {
				txId = monitoring.GenerateTxId(meta, controller.name, txId)
				metaName = meta.GetName()
				metaNamespace = meta.GetNamespace()
			}
			ctx = context.WithValue(ctx, "txId", txId)
			ctxLogger := logrus.WithFields(logrus.Fields{
				"op":         operationInformerEvents,
				"name":       metaName,
				"namespace":  metaNamespace,
				"controller": controller.name,
				"cluster":    controller.cluster,
				"txId":       txId,
			})

			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				ctxLogger.Infof(ControllerLogFormat, taskAddEventToQueue, controller.queue.Len(), Update+" Event")
				// Check if the event has already been processed or the resource version
				// has changed. If either the event has not been processed yet or the
				// resource version has changed only then add it to the queue

				status, err := controller.delegator.GetProcessItemStatus(newObj)
				if err != nil {
					ctxLogger.Errorf(err.Error())
				}
				controller.delegator.LogValueOfAdmiralIoIgnore(newObj)
				latestObj, isVersionChanged := checkIfResourceVersionHasIncreased(ctxLogger, ctx, oldObj, newObj, delegator)
				txId, ctxLogger = updateTxId(newObj, latestObj, txId, ctxLogger, controller)

				if status == types.NotProcessed || isVersionChanged {
					ctxLogger.Infof(ControllerLogFormat, taskAddEventToQueue, controller.queue.Len(),
						fmt.Sprintf("version changed=%v", isVersionChanged))
					controller.queue.Add(
						InformerCacheObj{
							key:       key,
							eventType: Update,
							obj:       latestObj,
							oldObj:    oldObj,
							txId:      txId,
							ctxLogger: ctxLogger,
						})
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			var (
				txId = uuid.NewString()
			)
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				meta, ok := obj.(metav1.Object)
				var metaName, metaNamespace string
				if ok && meta != nil && meta.GetResourceVersion() != "" {
					txId = monitoring.GenerateTxId(meta, controller.name, txId)
					metaName = meta.GetName()
					metaNamespace = meta.GetNamespace()
				}
				ctxLogger := logrus.WithFields(logrus.Fields{
					"op":         operationInformerEvents,
					"name":       metaName,
					"namespace":  metaNamespace,
					"controller": controller.name,
					"cluster":    controller.cluster,
					"txId":       txId,
				})
				ctxLogger.Infof(ControllerLogFormat, taskAddEventToQueue, controller.queue.Len(), Delete+" Event")
				controller.queue.Add(
					InformerCacheObj{
						key:       key,
						eventType: Delete,
						obj:       obj,
						txId:      txId,
						ctxLogger: ctxLogger,
					})
			}
		},
	})
	go controller.Run(stopCh)
	return controller
}

func updateTxId(
	newObj, latestObj interface{},
	txId string,
	ctxLogger *logrus.Entry,
	controller Controller) (string, *logrus.Entry) {
	lMeta, ok := latestObj.(metav1.Object)
	if ok && lMeta.GetResourceVersion() != "" {
		nMeta, ok := newObj.(metav1.Object)
		if ok && nMeta.GetResourceVersion() != lMeta.GetResourceVersion() {
			txId = monitoring.GenerateTxId(lMeta, controller.name, txId)
			ctxLogger = logrus.WithFields(logrus.Fields{
				"op":         operationInformerEvents,
				"controller": controller.name,
				"cluster":    controller.cluster,
				"txId":       txId,
			})
		}
	}
	return txId, ctxLogger
}

// Run starts the controller until it receives a message over stopCh
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.queue.ShutDown()

	logrus.Infof("Starting controller=%v cluster=%v", c.name, c.cluster)

	go c.informer.Run(stopCh)

	// Wait for the caches to be synced before starting workers
	logrus.Infof("Waiting for informer caches to sync for controller=%v cluster=%v", c.name, c.cluster)
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync for controller=%v cluster=%v", c.name, c.cluster))
		return
	}
	logrus.Infof("Informer caches synced for controller=%v cluster=%v, current keys=%v", c.name, c.cluster, c.informer.GetStore().ListKeys())
	wait.Until(c.runWorker, 5*time.Second, stopCh)
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
	logrus.Errorf("Shutting Down controller=%v cluster=%v", c.name, c.cluster)
}

func (c *Controller) processNextItem() bool {
	item, quit := c.queue.Get()
	if item == nil || quit {
		return false
	}
	logrus.Infof(LogQueueFormat, taskGetEventFromQueue, c.name, c.cluster, c.queue.Len(), "current queue length")
	defer c.queue.Done(item)
	informerCache, ok := item.(InformerCacheObj)
	if !ok {
		return true
	}
	var (
		txId         string
		err          error
		processEvent = true
		ctx          = context.Background()
	)

	txId = informerCache.txId
	ctx = context.WithValue(ctx, "txId", txId)
	ctxLogger := informerCache.ctxLogger
	if c.queue.NumRequeues(item) > 0 {
		ctxLogger.Infof(ControllerLogFormat, taskRequeueAttempt, c.queue.Len(),
			fmt.Sprintf("retryCount=%d", c.queue.NumRequeues(item)))
		processEvent = shouldRetry(ctxLogger, ctx, informerCache.obj, c.delegator)
	}
	if processEvent {
		err = c.processItem(item.(InformerCacheObj))
	} else {
		ctxLogger.Infof(ControllerLogFormat, taskRequeueAttempt, c.queue.Len(),
			"stale event will not be retried. newer event was already processed")
		c.queue.Forget(item)
		return true
	}

	if err == nil {
		// No error, forget item
		c.queue.Forget(item)
	} else if c.queue.NumRequeues(item) < maxRetries {
		ctxLogger.Errorf(ControllerLogFormat, taskRequeueAttempt, c.queue.Len(), "checking if event is eligible for requeueing. error="+err.Error())
		processRetry := shouldRetry(ctxLogger, ctx, item.(InformerCacheObj).obj, c.delegator)
		if processRetry {
			ctxLogger.Infof(ControllerLogFormat, taskRequeueAttempt, c.queue.Len(),
				fmt.Sprintf("event is eligible for retry. retryCount=%v", c.queue.NumRequeues(item)))
			c.queue.AddRateLimited(item)
		} else {
			ctxLogger.Infof(ControllerLogFormat, taskRequeueAttempt, c.queue.Len(),
				fmt.Sprintf("event is not eligible for retry. forgetting event. retryCount=%v", c.queue.NumRequeues(item)))
			c.queue.Forget(item)
		}
	} else {
		ctxLogger.Errorf(ControllerLogFormat, taskGivingUpEvent, c.queue.Len(), "not requeueing. error="+err.Error())
		c.queue.Forget(item)
		// If the controller is not able to process the event even after retries due to
		// errors we mark it as NotProcessed
		c.delegator.UpdateProcessItemStatus(item.(InformerCacheObj).obj, types.NotProcessed)
		runtime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(informerCacheObj InformerCacheObj) error {
	var (
		ctx       = context.Background()
		txId      = informerCacheObj.txId
		ctxLogger = informerCacheObj.ctxLogger
	)
	ctxLogger.Infof(ControllerLogFormat, taskSendEventToDelegator, c.queue.Len(), "processing event")
	// defer util.LogElapsedTimeController(
	// ctxLogger, fmt.Sprintf(ControllerLogFormat, taskSendEventToDelegator, c.queue.Len(), "processingTime"))()
	ctx = context.WithValue(ctx, "txId", txId)
	ctx = context.WithValue(ctx, "controller", c.name)
	var err error
	if informerCacheObj.eventType == Delete {
		err = c.delegator.Deleted(ctx, informerCacheObj.obj)
	} else if informerCacheObj.eventType == Update {
		err = c.delegator.Updated(ctx, informerCacheObj.obj, informerCacheObj.oldObj)
	} else if informerCacheObj.eventType == Add {
		err = c.delegator.Added(ctx, informerCacheObj.obj)
	}

	// processItemStatus is set to:
	// 1. Processed only if there are no errors and Admiral is not in read only mode
	// 2. ProcessingInProgress if not in read only mode but there are errors
	// 3. NotProcessed if it is in read only mode
	processItemStatus := types.NotProcessed
	if err == nil {
		processItemStatus = types.Processed
	}
	ctxLogger.Infof(ControllerLogFormat, taskReceiveEventFromDelegator, c.queue.Len(), "status="+processItemStatus)
	c.delegator.UpdateProcessItemStatus(informerCacheObj.obj, processItemStatus)
	return err
}

// checkIfResourceVersionHasIncreased compares old object, with the new obj
// and returns true, along with the object which should be processed.
// It returns true when:
//  1. new version > old version
//  2. new version < old version:
//     When new version had been reset after reaching the max value
//     which could be assigned to it.
//
// For all other cases it returns false, which signals that the object
// should not be processed, because:
// 1. It was already processed
// 2. It is an older object
func checkIfResourceVersionHasIncreased(ctxLogger *logrus.Entry, ctx context.Context, oldObj, newObj interface{}, delegator Delegator) (interface{}, bool) {
	oldObjMeta, oldOk := oldObj.(metav1.Object)
	newObjMeta, newOk := newObj.(metav1.Object)

	if oldOk && newOk && oldObjMeta.GetResourceVersion() == newObjMeta.GetResourceVersion() {
		return oldObj, false
	}
	if oldOk && newOk && oldObjMeta.GetResourceVersion() > newObjMeta.GetResourceVersion() {
		if reflect.ValueOf(delegator).IsNil() {
			return oldObj, true
		}
		// if old version is > new version then this could be due to:
		// 1. An old object was requeued because of retry, which now comes as new object
		// 2. The new object version is lower than old object version because the
		//    version had reached the maximum value, and was reset to a lower
		//    value by kubernetes
		ctxLogger.Infof("task=CheckIfResourceVersionHasIncreased message=new resource version is smaller than old resource version, checking if this is due to resourceVersion wrapping around")
		var (
			maxRetry  = 5
			latestObj interface{}
			err       error
		)

		err = util.RetryWithBackOff(ctx, func() error {
			latestObj, err = delegator.Get(ctx, false, newObj)
			return err
		}, maxRetry)
		if err != nil {
			ctxLogger.Errorf("task=CheckIfResourceVersionHasIncreased message=unable to fetch latest object from kubernetes after %d retries, giving up querying obj from API server, old obj=%+v, new obj=%+v",
				maxRetry, oldObjMeta, latestObj)
			return newObj, true
		}
		// event 1 ==> processed
		// event 2, 3 ==> happen simultaneously, 3 is expected to be final state
		// event 3 ==> processed
		// event 2 ==> ready to be processed ==> this event is in the new object, passed into this function
		// the below check will ensure that this is the case
		// as it fetches the latest object from kubernetes, and finds it was
		// event 3, which is nothing but old object
		latestObjMeta, latestOk := latestObj.(metav1.Object)
		if latestOk && oldObjMeta.GetResourceVersion() == latestObjMeta.GetResourceVersion() {
			ctxLogger.Infof("task=CheckIfResourceVersionHasIncreased message=not processing resource version=%v, because it is stale, and was added to the queue due to a retry. version=%v was already processed",
				newObjMeta.GetResourceVersion(),
				latestObjMeta.GetResourceVersion())
			return oldObj, false
		}
		ctxLogger.Infof("task=CheckIfResourceVersionHasIncreased message=new version is less than old version, which is because it was wrapped around by kubernetes, after reaching max allowable value")
		return latestObj, true
	}
	return newObj, true
}

func shouldRetry(ctxLogger *logrus.Entry, ctx context.Context, obj interface{}, delegator Delegator) bool {
	objMeta, ok := obj.(metav1.Object)
	if ok {
		if reflect.ValueOf(objMeta).IsNil() || reflect.ValueOf(delegator).IsNil() {
			return true
		}
		objFromCache, err := delegator.Get(ctx, true, obj)
		if err != nil {
			ctxLogger.Errorf("task=shouldRetry message=unable to fetch latest object from cache, obj received=%+v", objMeta)
			return true
		}
		latestObjMeta, latestOk := objFromCache.(metav1.Object)
		if !latestOk || reflect.ValueOf(latestObjMeta).IsNil() {
			ctxLogger.Errorf("task=shouldRetry message=unable to cast latest object from cache to metav1 object, obj received=%+v", objMeta)
			return true
		}
		// event 1 ==> processed
		// event 2 ==> failed
		// event 3 ==> processed
		// event 2 ==> requeued ==> this event is in the object, passed into this function
		// the below check will ensure that this is the case
		// as it fetches the latest object from cache, and finds it was
		// event 3, which is a newer event

		if objMeta.GetResourceVersion() < latestObjMeta.GetResourceVersion() {
			ctxLogger.Infof("task=shouldRetry message=not processing resource version=%v, because it is stale, and was added to the queue due to a retry. version=%v was already processed",
				objMeta.GetResourceVersion(), latestObjMeta.GetResourceVersion())
			return false
		}
		// TODO: Wrap around check- make Kube API server call to get the latest object and ensure
		// we do not retry when the resource version has been wrapped around. Implementation similar to checkIfResourceVersionHasIncreased.
	}
	ctxLogger.Errorf("task=shouldRetry message=obj parsed=%v, retrying object, obj received=%+v", ok, objMeta)
	return true
}
