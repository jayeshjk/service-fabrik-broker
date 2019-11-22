/*
Copyright 2018 The Service Fabrik Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sfserviceinstance

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"

	osbv1alpha1 "github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/api/osb/v1alpha1"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/internal/config"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/internal/properties"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/internal/resources"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/cluster/registry"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/constants"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/errors"
	"github.com/cloudfoundry-incubator/service-fabrik-broker/interoperator/pkg/utils"

	"github.com/go-logr/logr"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

var ownClusterID string

// ReconcileSFServiceInstance reconciles a SFServiceInstance object
type ReconcileSFServiceInstance struct {
	client.Client
	uncachedClient  client.Client
	Log             logr.Logger
	scheme          *runtime.Scheme
	clusterRegistry registry.ClusterRegistry
	resourceManager resources.ResourceManager
}

// Reconcile reads that state of the cluster for a SFServiceInstance object and makes changes based on the state read
// and what is in the SFServiceInstance.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=osb.servicefabrik.io,resources=*,verbs=*
// +kubebuilder:rbac:groups=deployment.servicefabrik.io,resources=*,verbs=*
// +kubebuilder:rbac:groups=kubernetes.sapcloud.io,resources=*,verbs=*
// +kubebuilder:rbac:groups=kubedb.com,resources=Postgres,verbs=*
// +kubebuilder:rbac:groups=,resources=configmap,verbs=*
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=*
// TODO dynamically setup rbac rules
func (r *ReconcileSFServiceInstance) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("sfserviceinstance", req.NamespacedName)

	// Fetch the ServiceInstance instance
	instance := &osbv1alpha1.SFServiceInstance{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			log.Info("instance deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return r.handleError(instance, ctrl.Result{}, err, "", 0)
	}

	serviceID := instance.Spec.ServiceID
	planID := instance.Spec.PlanID
	instanceID := instance.GetName()
	bindingID := ""
	state := instance.GetState()

	if state == "succeeded" || state == "failed" {
		return ctrl.Result{}, nil
	}

	// Fetch again using uncachedClient to read the state again
	err = r.uncachedClient.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			log.Info("instance deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return r.handleError(instance, ctrl.Result{}, err, state, 0)
	}
	state = instance.GetState()
	labels := instance.GetLabels()
	lastOperation, ok := labels[constants.LastOperationKey]
	if !ok {
		lastOperation = "in_queue"
	}

	clusterID, err := instance.GetClusterID()
	if err != nil {
		if errors.SFServiceInstanceNotFound(err) || errors.ClusterIDNotSet(err) {
			log.Info("clusterID not set. Ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get clusterID")
		return r.handleError(instance, ctrl.Result{}, err, state, 0)
	}
	if clusterID != ownClusterID {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileFinalizers(instance, 0); err != nil {
		return r.handleError(instance, ctrl.Result{Requeue: true}, nil, "", 0)
	}

	targetClient := r

	if state == "delete" && !instance.GetDeletionTimestamp().IsZero() {
		// The object is being deleted
		// so lets handle our external dependency
		remainingResource, err := r.resourceManager.DeleteSubResources(targetClient, instance.Status.Resources)
		if err != nil {
			log.Error(err, "Delete sub resources failed")
			return r.handleError(instance, ctrl.Result{}, err, state, 0)
		}
		err = r.setInProgress(req.NamespacedName, state, remainingResource, 0)
		if err != nil {
			return r.handleError(instance, ctrl.Result{}, err, state, 0)
		}
		lastOperation = state
	} else if state == "in_queue" || state == "update" {
		expectedResources, err := r.resourceManager.ComputeExpectedResources(r, instanceID, bindingID, serviceID, planID, osbv1alpha1.ProvisionAction, instance.GetNamespace())
		if err != nil {
			return r.handleError(instance, ctrl.Result{}, err, state, 0)
		}

		err = r.resourceManager.SetOwnerReference(instance, expectedResources, r.scheme)
		if err != nil {
			return r.handleError(instance, ctrl.Result{}, err, state, 0)
		}

		resourceRefs, err := r.resourceManager.ReconcileResources(r, targetClient, expectedResources, instance.Status.Resources)
		if err != nil {
			log.Error(err, "ReconcileResources failed")
			return r.handleError(instance, ctrl.Result{}, err, state, 0)
		}
		err = r.setInProgress(req.NamespacedName, state, resourceRefs, 0)
		if err != nil {
			return r.handleError(instance, ctrl.Result{}, err, state, 0)
		}
		lastOperation = state
	}

	err = r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		return r.handleError(instance, ctrl.Result{}, err, "", 0)
	}
	state = instance.GetState()
	labels = instance.GetLabels()
	lastOperation, ok = labels[constants.LastOperationKey]
	if !ok {
		lastOperation = "in_queue"
	}

	if state == "in progress" {
		if lastOperation == "delete" {
			if err := r.updateDeprovisionStatus(targetClient, instance, 0); err != nil {
				return r.handleError(instance, ctrl.Result{}, err, lastOperation, 0)
			}
		} else if lastOperation == "in_queue" || lastOperation == "update" {
			err = r.updateStatus(targetClient, instance, 0)
			if err != nil {
				return r.handleError(instance, ctrl.Result{}, err, lastOperation, 0)
			}
		}
	}
	return r.handleError(instance, ctrl.Result{}, nil, lastOperation, 0)
}

func (r *ReconcileSFServiceInstance) reconcileFinalizers(object *osbv1alpha1.SFServiceInstance, retryCount int) error {
	ctx := context.Background()

	objectID := object.GetName()
	namespace := object.GetNamespace()
	// Fetch object again before updating
	namespacedName := types.NamespacedName{
		Name:      objectID,
		Namespace: namespace,
	}
	log := r.Log.WithValues("sfserviceinstance", namespacedName)

	err := r.Get(ctx, namespacedName, object)
	if err != nil {
		if retryCount < constants.ErrorThreshold {
			log.Info("Retrying", "function", "reconcileFinalizers", "retryCount", retryCount+1, "objectID", objectID)
			return r.reconcileFinalizers(object, retryCount+1)
		}
		log.Error(err, "failed to fetch object", "objectID", objectID)
		return err
	}
	if object.GetDeletionTimestamp().IsZero() {
		if !utils.ContainsString(object.GetFinalizers(), constants.FinalizerName) {
			// The object is not being deleted, so if it does not have our finalizer,
			// then lets add the finalizer and update the object.
			object.SetFinalizers(append(object.GetFinalizers(), constants.FinalizerName))
			if err := r.Update(ctx, object); err != nil {
				if retryCount < constants.ErrorThreshold {
					log.Info("Retrying", "function", "reconcileFinalizers", "retryCount", retryCount+1, "objectID", objectID)
					return r.reconcileFinalizers(object, retryCount+1)
				}
				log.Error(err, "failed to add finalizer", "objectID", objectID)
				return err
			}
			log.Info("added finalizer", "objectID", objectID)
		}
	}
	return nil
}

func (r *ReconcileSFServiceInstance) setInProgress(namespacedName types.NamespacedName, state string, resources []osbv1alpha1.Source, retryCount int) error {
	ctx := context.Background()
	log := r.Log.WithValues("sfserviceinstance", namespacedName)

	if state == "in_queue" || state == "update" || state == "delete" {
		instance := &osbv1alpha1.SFServiceInstance{}
		err := r.Get(ctx, namespacedName, instance)
		if err != nil {
			if retryCount < constants.ErrorThreshold {
				log.Info("Retrying", "function", "setInProgress", "retryCount", retryCount+1, "objectID", namespacedName.Name)
				return r.setInProgress(namespacedName, state, resources, retryCount+1)
			}
			log.Error(err, "Updating status to in progress failed", "instanceId", namespacedName.Name)
			return err
		}
		instance.SetState("in progress")
		labels := instance.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[constants.LastOperationKey] = state
		instance.SetLabels(labels)
		instance.Status.Resources = resources
		err = r.Update(ctx, instance)
		if err != nil {
			if retryCount < constants.ErrorThreshold {
				log.Info("Retrying", "function", "setInProgress", "retryCount", retryCount+1, "objectID", namespacedName.Name)
				return r.setInProgress(namespacedName, state, resources, retryCount+1)
			}
			log.Error(err, "Updating status to in progress failed", "instanceId", namespacedName.Name)
			return err
		}
		log.Info("Updated status to in progress", "operation", state, "instanceId", namespacedName.Name)
	}
	return nil
}

func (r *ReconcileSFServiceInstance) updateDeprovisionStatus(targetClient client.Client, instance *osbv1alpha1.SFServiceInstance, retryCount int) error {
	ctx := context.Background()

	serviceID := instance.Spec.ServiceID
	planID := instance.Spec.PlanID
	instanceID := instance.GetName()
	bindingID := ""
	namespace := instance.GetNamespace()

	log := r.Log.WithValues("sfserviceinstance", instanceID)

	computedStatus, err := r.resourceManager.ComputeStatus(r, targetClient, instanceID, bindingID, serviceID, planID, osbv1alpha1.ProvisionAction, namespace)
	if err != nil && !errors.NotFound(err) {
		log.Error(err, "ComputeStatus failed for deprovision", "instanceId", instanceID)
		return err
	}

	if errors.NotFound(err) && computedStatus == nil {
		computedStatus = &properties.Status{}
		computedStatus.Deprovision.State = instance.GetState()
		computedStatus.Deprovision.Error = err.Error()
	}

	// Fetch object again before updating status
	namespacedName := types.NamespacedName{
		Name:      instanceID,
		Namespace: namespace,
	}
	err = r.Get(ctx, namespacedName, instance)
	if err != nil {
		log.Error(err, "Failed to get instance", "instanceId", instanceID)
		return err
	}

	updateRequired := false
	updatedStatus := instance.Status.DeepCopy()
	updatedStatus.State = computedStatus.Deprovision.State
	updatedStatus.Error = computedStatus.Deprovision.Error
	updatedStatus.Description = computedStatus.Deprovision.Response

	remainingResource := []osbv1alpha1.Source{}
	for _, subResource := range instance.Status.Resources {
		resource := &unstructured.Unstructured{}
		resource.SetKind(subResource.Kind)
		resource.SetAPIVersion(subResource.APIVersion)
		resource.SetName(subResource.Name)
		resource.SetNamespace(subResource.Namespace)
		namespacedName := types.NamespacedName{
			Name:      resource.GetName(),
			Namespace: resource.GetNamespace(),
		}
		err := targetClient.Get(ctx, namespacedName, resource)
		if !apiErrors.IsNotFound(err) {
			remainingResource = append(remainingResource, subResource)
		}
	}
	updatedStatus.Resources = remainingResource
	if !reflect.DeepEqual(&instance.Status, updatedStatus) {
		updatedStatus.DeepCopyInto(&instance.Status)
		updateRequired = true
	}

	if instance.GetState() == "succeeded" || len(remainingResource) == 0 {
		// remove our finalizer from the list and update it.
		log.Info("Removing finalizer", "instance", instanceID)
		instance.SetFinalizers(utils.RemoveString(instance.GetFinalizers(), constants.FinalizerName))
		instance.SetState("succeeded")
		updateRequired = true
	}

	if updateRequired {
		log.Info("Updating deprovision status from template", "instance", namespacedName.Name)
		if err := r.Update(ctx, instance); err != nil {
			if retryCount < constants.ErrorThreshold {
				log.Info("Retrying", "function", "updateDeprovisionStatus", "retryCount", retryCount+1, "instanceID", instanceID)
				return r.updateDeprovisionStatus(targetClient, instance, retryCount+1)
			}
			log.Error(err, "failed to update deprovision status", "instance", instanceID)
			return err
		}
	}
	return nil
}

func (r *ReconcileSFServiceInstance) updateStatus(targetClient client.Client, instance *osbv1alpha1.SFServiceInstance, retryCount int) error {
	serviceID := instance.Spec.ServiceID
	planID := instance.Spec.PlanID
	instanceID := instance.GetName()
	bindingID := ""
	namespace := instance.GetNamespace()

	ctx := context.Background()
	log := r.Log.WithValues("sfserviceinstance", instanceID)

	computedStatus, err := r.resourceManager.ComputeStatus(r, targetClient, instanceID, bindingID, serviceID, planID, osbv1alpha1.ProvisionAction, namespace)
	if err != nil {
		log.Error(err, "Compute status failed", "instance", instanceID)
		return err
	}

	// Fetch object again before updating status
	namespacedName := types.NamespacedName{
		Name:      instanceID,
		Namespace: namespace,
	}
	err = r.Get(ctx, namespacedName, instance)
	if err != nil {
		log.Error(err, "failed to fetch instance", "instance", instanceID)
		return err
	}
	updatedStatus := instance.Status.DeepCopy()
	updatedStatus.State = computedStatus.Provision.State
	updatedStatus.Error = computedStatus.Provision.Error
	updatedStatus.Description = computedStatus.Provision.Response
	updatedStatus.DashboardURL = computedStatus.Provision.DashboardURL

	if !reflect.DeepEqual(&instance.Status, updatedStatus) {
		updatedStatus.DeepCopyInto(&instance.Status)
		log.Info("Updating provision status from template", "instance", namespacedName.Name)
		err = r.Update(ctx, instance)
		if err != nil {
			if retryCount < constants.ErrorThreshold {
				log.Info("Retrying", "function", "updateStatus", "retryCount", retryCount+1, "instanceID", instanceID)
				return r.updateStatus(targetClient, instance, retryCount+1)
			}
			log.Error(err, "failed to update status", "instanceId", instanceID)
			return err
		}
	}
	return nil
}

func (r *ReconcileSFServiceInstance) handleError(object *osbv1alpha1.SFServiceInstance, result ctrl.Result, inputErr error, lastOperation string, retryCount int) (ctrl.Result, error) {
	objectID := object.GetName()
	namespace := object.GetNamespace()
	// Fetch object again before updating
	namespacedName := types.NamespacedName{
		Name:      objectID,
		Namespace: namespace,
	}

	ctx := context.Background()
	log := r.Log.WithValues("objectID", objectID)

	err := r.Get(ctx, namespacedName, object)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			return result, inputErr
		}
		if retryCount < constants.ErrorThreshold {
			log.Info("Retrying", "function", "handleError", "retryCount", retryCount+1, "lastOperation", lastOperation, "err", inputErr, "objectID", objectID)
			return r.handleError(object, result, inputErr, lastOperation, retryCount+1)
		}
		log.Error(err, "failed to fetch object", "objectID", objectID)
		return result, inputErr
	}

	labels := object.GetLabels()
	var count int64

	if labels == nil {
		labels = make(map[string]string)
	}

	countString, ok := labels[constants.ErrorCountKey]
	if !ok {
		count = 0
	} else {
		i, err := strconv.ParseInt(countString, 10, 64)
		if err != nil {
			count = 0
		} else {
			count = i
		}
	}

	if inputErr == nil {
		if count == 0 {
			//No change for count
			return result, inputErr
		}
		count = 0
	} else {
		count++
	}

	if count > constants.ErrorThreshold {
		log.Error(inputErr, "Retry threshold reached. Ignoring error", "objectID", objectID)
		object.Status.State = "failed"
		object.Status.Error = fmt.Sprintf("Retry threshold reached for %s.\n%s", objectID, inputErr.Error())
		object.Status.Description = "Service Broker Error, status code: ETIMEDOUT, error code: 10008"
		if lastOperation != "" {
			labels[constants.LastOperationKey] = lastOperation
			object.SetLabels(labels)
		}
		err := r.Update(ctx, object)
		if err != nil {
			if retryCount < constants.ErrorThreshold {
				log.Info("Retrying", "function", "handleError", "retryCount", retryCount+1, "lastOperation", lastOperation, "err", inputErr, "objectID", objectID)
				return r.handleError(object, result, inputErr, lastOperation, retryCount+1)
			}
			log.Error(err, "Failed to set state to failed", "objectID", objectID)
		}
		return result, nil
	}

	labels[constants.ErrorCountKey] = strconv.FormatInt(count, 10)
	object.SetLabels(labels)
	err = r.Update(ctx, object)
	if err != nil {
		if retryCount < constants.ErrorThreshold {
			log.Info("Retrying", "function", "handleError", "retryCount", retryCount+1, "lastOperation", lastOperation, "err", inputErr, "objectID", objectID)
			return r.handleError(object, result, inputErr, lastOperation, retryCount+1)
		}
		log.Error(err, "Failed to update error count label", "objectID", objectID, "count", count)
	}
	return result, inputErr
}

// SetupWithManager registers the SFServiceInstance Controller with manager
// and setups the watches.
func (r *ReconcileSFServiceInstance) SetupWithManager(mgr ctrl.Manager) error {
	r.scheme = mgr.GetScheme()

	if r.Log == nil {
		r.Log = ctrl.Log.WithName("provisioners").WithName("instance")
	}
	if r.clusterRegistry == nil {
		clusterRegistry, err := registry.New(mgr.GetConfig(), mgr.GetScheme(), mgr.GetRESTMapper())
		if err != nil {
			return err
		}
		r.clusterRegistry = clusterRegistry
	}

	if r.resourceManager == nil {
		r.resourceManager = resources.New()
	}

	if r.uncachedClient == nil {
		uncachedClient, err := client.New(mgr.GetConfig(), client.Options{
			Scheme: mgr.GetScheme(),
			Mapper: mgr.GetRESTMapper(),
		})
		if err != nil {
			return err
		}
		r.uncachedClient = uncachedClient
	}

	cfgManager, err := config.New(mgr.GetConfig(), mgr.GetScheme(), mgr.GetRESTMapper())
	if err != nil {
		return err
	}
	interoperatorCfg := cfgManager.GetConfig()

	ownClusterID = os.Getenv(constants.OwnClusterIDEnvKey)
	if ownClusterID == "" {
		ownClusterID = constants.DefaultMasterClusterID
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		Named("instance").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: interoperatorCfg.InstanceWorkerCount,
		}).
		For(&osbv1alpha1.SFServiceInstance{})

	// TODO dynamically setup rbac rules and watches
	subresources := make([]runtime.Object, len(interoperatorCfg.InstanceContollerWatchList))
	for i, gvk := range interoperatorCfg.InstanceContollerWatchList {
		object := &unstructured.Unstructured{}
		object.SetKind(gvk.GetKind())
		object.SetAPIVersion(gvk.GetAPIVersion())
		subresources[i] = object
	}

	for _, subresource := range subresources {
		builder = builder.Owns(subresource)
	}

	return builder.Complete(r)
}
