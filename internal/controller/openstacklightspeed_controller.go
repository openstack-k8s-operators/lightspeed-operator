/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
)

// DynamicWatchCRD maps GroupVersionKinds to a flag indicating whether a
// controller-runtime watch is currently registered. The flag is set to true
// when a watch is registered and reset to false when the CRD is removed and
// the broken informer is cleaned up via cache.RemoveInformer. CRDs in this
// map do not need to exist at operator startup.
type DynamicWatchCRD map[schema.GroupVersionKind]*atomic.Bool

// OpenStackLightspeedReconciler reconciles a OpenStackLightspeed object
type OpenStackLightspeedReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Kclient    kubernetes.Interface
	controller controller.Controller
	Cache      cache.Cache

	// DynamicWatchCRD contains the list of CRDs that the operator should monitor.
	// These CRDs do not need to exist when the operator starts. Once the operator
	// detects that a CRD exists, it automatically registers a watch for it.
	DynamicWatchCRD DynamicWatchCRD
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *OpenStackLightspeedReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OpenStackLightspeed")
}

// +kubebuilder:rbac:groups=lightspeed.openstack.org,resources=openstacklightspeeds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lightspeed.openstack.org,resources=openstacklightspeeds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lightspeed.openstack.org,resources=openstacklightspeeds/finalizers,verbs=update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;patch;update;delete;deletecollection
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;patch;update;delete;deletecollection
// +kubebuilder:rbac:groups=operators.coreos.com,resources=clusterserviceversions,verbs=get;list;watch
// +kubebuilder:rbac:groups=operators.coreos.com,resources=clusterserviceversions,namespace=openstack-lightspeed,verbs=update;patch;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch
// SAR role escalation: the operator creates a ClusterRole granting pull-secret GET,
// so it must hold that permission itself (K8s RBAC escalation prevention).
// +kubebuilder:rbac:groups="",resources=secrets,resourceNames=pull-secret,verbs=get
// Cross-namespace secrets: GET reads OSCP secrets (CA bundle, clouds.yaml, secure.yaml needed to create user);
// CREATE creates lightspeed-password in the OSCP namespace (resourceNames can't restrict create);
// UPDATE adds/removes finalizers on the AC secret (dynamic name set by keystone-operator).
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;create;update
// lightspeed-password lifecycle: update credentials on rotation, delete on CR teardown.
// +kubebuilder:rbac:groups="",resources=secrets,resourceNames=lightspeed-password,verbs=get;update;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core.openstack.org,resources=openstackcontrolplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapplicationcredentials/status,verbs=get
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,namespace=openstack-lightspeed,verbs=get;list;watch;create;patch;update
// +kubebuilder:rbac:groups=apps,resources=deployments,namespace=openstack-lightspeed,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,namespace=openstack-lightspeed,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=secrets,namespace=openstack-lightspeed,verbs=get;list;watch;create;patch;update;delete;deletecollection
// +kubebuilder:rbac:groups="",resources=services,namespace=openstack-lightspeed,verbs=get;list;watch;create;patch;update;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,namespace=openstack-lightspeed,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=console.openshift.io,resources=consoleplugins,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.openshift.io,resources=consoles,verbs=watch;list;get;update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,namespace=openstack-lightspeed,verbs=get;list;watch;create;patch;update

func (r *OpenStackLightspeedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, e error) {
	Log := r.GetLogger(ctx)
	Log.Info("OpenStackLightspeed Reconciling")

	instance := &apiv1beta1.OpenStackLightspeed{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("OpenStackLightspeed CR not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	helper, err := common_helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.WatchDynamicCRD(ctx, helper)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Save a copy of the conditions so that we can restore the LastTransitionTime
	// when a condition's state doesn't change.
	savedConditions := instance.Status.Conditions.DeepCopy()

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// Don't update the status, if reconciler Panics
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}

		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}

		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			return
		}

		// Poll when any dynamically watched CRD is missing, or when
		// rhos_mcps is enabled — the cache-based watch only covers
		// the operator namespace, so polling detects cross-namespace
		// OpenStackControlPlane changes (readiness, CA rotations, etc.).
		if result.RequeueAfter == 0 {
			needsPoll := false
			for _, seen := range r.DynamicWatchCRD {
				if !seen.Load() {
					needsPoll = true
					break
				}
			}
			if !needsPoll {
				if enabled, _ := isRHOSMCPEnabled(instance); enabled {
					needsPoll = true
				}
			}
			if needsPoll {
				result.RequeueAfter = ResourceCreationTimeout
			}
		}
	}()

	cl := condition.CreateList(
		condition.UnknownCondition(
			apiv1beta1.OpenStackLightspeedReadyCondition,
			condition.InitReason,
			apiv1beta1.OpenStackLightspeedReadyInitMessage,
		),
		condition.UnknownCondition(
			apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
			condition.InitReason,
			apiv1beta1.OpenStackLightspeedMCPServerInitMessage,
		),
	)

	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	if !instance.DeletionTimestamp.IsZero() {
		if err := r.reconcileDelete(ctx, helper, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
		return ctrl.Result{}, nil
	}

	if instance.Spec.MaxTokensForResponse == 0 {
		instance.Spec.MaxTokensForResponse = apiv1beta1.OpenStackLightspeedDefaultValues.MaxTokensForResponse
	}

	// Log dev config parse errors so misconfigurations don't silently disable features.
	if _, err := parseDevConfig(instance); err != nil {
		Log.Error(err, "failed to parse dev config, ignoring")
	}

	reconcileTasks := []ReconcileTask{
		{Name: "MCPServer", Task: r.ReconcileMCPServerTask},
		{Name: "PostgresResources", Task: ReconcilePostgresResources},
		{Name: "PostgresDeployment", Task: ReconcilePostgresDeployment},
		{Name: "OKPDeployment", Task: ReconcileOKPDeployment},
		{Name: "LCoreResources", Task: ReconcileLCoreResources},
		{Name: "LCoreDeployment", Task: ReconcileLCoreDeployment},
		{Name: "ConsoleResources", Task: ReconcileConsoleResources},
		{Name: "ConsoleDeployment", Task: ReconcileConsoleDeployment},
	}

	if err := ReconcileTasks(helper, ctx, instance, reconcileTasks); err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			apiv1beta1.OpenStackLightspeedReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			apiv1beta1.DeploymentCheckFailedMessage,
			err.Error(),
		))
		return ctrl.Result{}, err
	}

	return r.reconcileStatus(ctx, helper, instance)
}

// reconcileDelete reconciles the deletion of OpenStackLightspeed instance
func (r *OpenStackLightspeedReconciler) reconcileDelete(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("OpenStackLightspeed Reconciling Delete")

	// Delete cluster-scoped resources using fail-fast pattern
	deletionTasks := []ReconcileTask{
		{Name: "DeleteConsolePlugin", Task: reconcileDeleteConsole},
		{Name: "DeleteSARClusterRoleBinding", Task: reconcileDeleteClusterRoleBindingByLabels},
		{Name: "DeleteSARClusterRole", Task: reconcileDeleteClusterRoleByLabels},
	}

	// Execute deletion tasks in order (fail-fast: stop on first error)
	if err := ReconcileTasksFailFast(helper, ctx, instance, deletionTasks); err != nil {
		Log.Error(err, "failed to delete cluster-scoped resources")
		return err
	}

	if err := r.reconcileDeleteOpenStackResources(ctx, helper, instance); err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())

	Log.Info("OpenStackLightspeed Reconciling Delete completed")
	return nil
}

func (r *OpenStackLightspeedReconciler) reconcileStatus(
	ctx context.Context,
	helper *common_helper.Helper,
	instance *apiv1beta1.OpenStackLightspeed,
) (ctrl.Result, error) {
	deployments := []string{
		PostgresDeploymentName,
		LCoreDeploymentName,
		ConsoleUIDeploymentName,
	}
	for _, deploymentName := range deployments {
		deployment, err := getDeployment(ctx, helper, deploymentName, instance.Namespace)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				apiv1beta1.OpenStackLightspeedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				apiv1beta1.DeploymentCheckFailedMessage,
				err.Error(),
			))
			return ctrl.Result{}, err
		}

		if !isDeploymentReady(deployment) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				apiv1beta1.OpenStackLightspeedReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				apiv1beta1.DeploymentsNotReadyMessage,
				deploymentName,
			))
			return ctrl.Result{RequeueAfter: ResourceCreationTimeout}, nil
		}
	}

	// Mark MCP server condition based on readiness (only when RHOS MCP is enabled;
	// when disabled the condition was already set in Reconcile).
	rhosMCPEnabled, err := isRHOSMCPEnabled(instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to parse dev config: %w", err)
	}
	if rhosMCPEnabled {
		if instance.Status.OpenStackReady {
			instance.Status.Conditions.MarkTrue(
				apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
				apiv1beta1.OpenStackLightspeedMCPServerDeployed,
			)
		} else {
			instance.Status.Conditions.MarkTrue(
				apiv1beta1.OpenStackLightspeedMCPServerReadyCondition,
				apiv1beta1.OpenStackLightspeedMCPServerWaitingOpenStack,
			)
		}
	}

	instance.Status.Conditions.MarkTrue(
		apiv1beta1.OpenStackLightspeedReadyCondition,
		apiv1beta1.OpenStackLightspeedReadyMessage,
	)

	helper.GetLogger().Info("OpenStackLightspeed Reconciled successfully")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenStackLightspeedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Use Build instead of Complete to get the controller reference needed by WatchDynamicCRD.
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&apiv1beta1.OpenStackLightspeed{}).
		Owns(&operatorsv1alpha1.ClusterServiceVersion{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&consolev1.ConsolePlugin{}).
		Watches(
			&corev1.PersistentVolumeClaim{},
			handler.EnqueueRequestsFromMapFunc(r.NotifyAllOpenStackLightspeeds),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.NotifyOpenStackLightspeedsByCAConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&apiextensionsv1.CustomResourceDefinition{},
			handler.EnqueueRequestsFromMapFunc(r.NotifyAllOpenStackLightspeeds),
			builder.WithPredicates(
				predicate.ResourceVersionChangedPredicate{},
				predicate.NewPredicateFuncs(func(obj client.Object) bool {
					for gvk := range r.DynamicWatchCRD {
						if obj.GetName() == GetCRDName(gvk) {
							return true
						}
					}
					return false
				}),
			),
		).
		Build(r)
	if err != nil {
		return err
	}
	r.controller = c
	return nil
}

// NotifyOpenStackLightspeedsByCAConfigMap watches ConfigMaps and triggers reconciliation when
// a user-provided CA ConfigMap (referenced by an OpenStackLightspeed CR) changes.
func (r *OpenStackLightspeedReconciler) NotifyOpenStackLightspeedsByCAConfigMap(ctx context.Context, obj client.Object) []ctrl.Request {
	var lightspeedList apiv1beta1.OpenStackLightspeedList
	if err := r.List(ctx, &lightspeedList, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, item := range lightspeedList.Items {
		if item.Spec.TLSCACertBundle == obj.GetName() {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: item.GetNamespace(),
					Name:      item.GetName(),
				},
			})
		}
	}
	return requests
}

// NotifyAllOpenStackLightspeeds returns a list of reconcile requests for all OpenStackLightspeed objects.
// For namespace-scoped resources (like InstallPlan), it lists in the same namespace as the triggering object.
// For cluster-scoped resources (like ClusterVersion), it lists in all namespaces the operator can access.
func (r *OpenStackLightspeedReconciler) NotifyAllOpenStackLightspeeds(ctx context.Context, obj client.Object) []ctrl.Request {
	var lightspeedList apiv1beta1.OpenStackLightspeedList
	var err error

	// For cluster-scoped resources (no namespace), list without namespace filter
	// The operator's cache is already restricted to the watch namespace, so this is safe
	if obj.GetNamespace() == "" {
		err = r.List(ctx, &lightspeedList)
	} else {
		err = r.List(ctx, &lightspeedList, client.InNamespace(obj.GetNamespace()))
	}

	if err != nil {
		return nil
	}

	requests := make([]ctrl.Request, 0, len(lightspeedList.Items))
	for _, item := range lightspeedList.Items {
		requests = append(requests, ctrl.Request{
			NamespacedName: client.ObjectKey{
				Namespace: item.GetNamespace(),
				Name:      item.GetName(),
			},
		})
	}

	return requests
}

// WatchDynamicCRD dynamically registers watches for resources whose CRDs are
// listed in r.DynamicWatchCRD. When a CRD is detected as established, it
// registers a watch and sets the flag to true. When a previously-watched CRD
// is removed, it cleans up the broken informer via cache.RemoveInformer and
// resets the flag to false. The next reconcile after CRD recreation will
// register a fresh watch.
func (r *OpenStackLightspeedReconciler) WatchDynamicCRD(
	ctx context.Context,
	helper *common_helper.Helper,
) error {
	Log := r.GetLogger(ctx)

	for gvk, seen := range r.DynamicWatchCRD {
		crdAvailable, err := IsCRDEstablished(ctx, helper, gvk)
		if err != nil {
			return err
		}

		if seen.Load() {
			if crdAvailable {
				continue
			}

			// CRD removed — clean up the broken informer so a fresh one
			// can be created when the CRD reappears.
			Log.Info("CRD removed, cleaning up stale informer", "crd", GetCRDName(gvk))
			obj := &uns.Unstructured{}
			obj.SetGroupVersionKind(gvk)
			if err := r.Cache.RemoveInformer(ctx, obj); err != nil {
				return fmt.Errorf("failed to remove informer for %s: %w", GetCRDName(gvk), err)
			}
			seen.Store(false)
			continue
		}

		if !crdAvailable {
			continue
		}

		GVKUnstructObj := &uns.Unstructured{}
		GVKUnstructObj.SetGroupVersionKind(gvk)
		err = r.controller.Watch(
			source.Kind(
				r.Cache,
				GVKUnstructObj,
				handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, o *uns.Unstructured) []ctrl.Request {
					return r.NotifyAllOpenStackLightspeeds(ctx, o)
				}),
				predicate.TypedResourceVersionChangedPredicate[*uns.Unstructured]{},
			),
		)
		if err != nil {
			return fmt.Errorf("failed to set up watch for %s: %w", GetCRDName(gvk), err)
		}

		seen.Store(true)
	}

	return nil
}
