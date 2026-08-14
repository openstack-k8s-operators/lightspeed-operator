/*
Copyright 2026

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
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"

	"github.com/openstack-k8s-operators/lightspeed-operator/internal/assistant"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
)

// openStackClientGVK is the GroupVersionKind of the OpenStackClient CR, which
// is owned and reconciled by openstack-operator (not this operator). The MCP
// sidecar it creates is read here via the unstructured/dynamic client so that
// lightspeed-operator does not take a Go module dependency on
// openstack-operator's API types (and their large transitive dependency graph).
//
// Cross-repo contract with openstack-operator's OpenStackClient controller
// (internal/controller/client/openstackclient_controller.go):
//   - spec.mcp.enabled must be true for the MCP sidecar/Service to exist.
//   - The MCP Service is named "<OpenStackClientRef>-mcp" in the same
//     namespace, listening on port 8080 at path "/openstack/".
//   - If spec.caBundleSecretName is set on the OpenStackClient, the MCP
//     endpoint is TLS (https) and that Secret contains the CA bundle
//     (key "tls-ca-bundle.pem") needed to validate it.
var openStackClientGVK = schema.GroupVersionKind{
	Group:   "client.openstack.org",
	Version: "v1beta1",
	Kind:    "OpenStackClient",
}

// OpenStackAssistantReconciler reconciles a OpenStackAssistant object
type OpenStackAssistantReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Kclient kubernetes.Interface
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *OpenStackAssistantReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("OpenStackAssistant")
}

// +kubebuilder:rbac:groups=lightspeed.openstack.org,resources=openstackassistants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lightspeed.openstack.org,resources=openstackassistants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lightspeed.openstack.org,resources=openstackassistants/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=client.openstack.org,resources=openstackclients,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:urls=/ls-access,verbs=get
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=nonroot-v2

// Reconcile -
func (r *OpenStackAssistantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &apiv1beta1.OpenStackAssistant{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			Log.Info("OpenStackAssistant CR not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	instance.Default()
	Log.Info("OpenStackAssistant CR values", "Name", instance.Name, "Namespace", instance.Namespace, "Image", instance.Spec.ContainerImage)

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// initialize status
	isNewInstance := instance.Status.Conditions == nil
	if isNewInstance {
		instance.Status.Conditions = condition.Conditions{}
	}

	savedConditions := instance.Status.Conditions.DeepCopy()

	defer func() {
		if r := recover(); r != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", r))
			panic(r)
		}
		condition.RestoreLastTransitionTimes(&instance.Status.Conditions, savedConditions)
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	cl := condition.CreateList(
		condition.UnknownCondition(apiv1beta1.OpenStackAssistantReadyCondition, condition.InitReason, apiv1beta1.OpenStackAssistantReadyInitMessage),
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

	if err := r.reconcileGooseServiceAccount(ctx, helper, instance); err != nil {
		return ctrl.Result{}, err
	}

	assistantLabels := map[string]string{
		// Must match the label openstack-operator's OpenStackClient
		// controller uses as the NetworkPolicy ingress-peer selector for
		// its MCP server (port 8080), so the assistant pod stays able to
		// reach it after moving to this operator.
		common.AppSelector: "openstackassistant",
	}

	configVars := make(map[string]env.Setter)

	// Validate lightspeed ProviderSecret
	_, providerSecretHash, err := secret.GetSecret(ctx, helper, instance.Spec.LightspeedStack.ProviderSecret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				apiv1beta1.OpenStackAssistantReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				apiv1beta1.OpenStackAssistantProviderSecretWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			apiv1beta1.OpenStackAssistantReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			apiv1beta1.OpenStackAssistantReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configVars[instance.Spec.LightspeedStack.ProviderSecret] = env.SetValue(providerSecretHash)

	// Validate optional CaBundleSecret
	if instance.Spec.LightspeedStack.CaBundleSecretName != "" {
		_, caBundleHash, err := secret.GetSecret(ctx, helper, instance.Spec.LightspeedStack.CaBundleSecretName, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					apiv1beta1.OpenStackAssistantReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					apiv1beta1.OpenStackAssistantReadyErrorMessage,
					"CA bundle secret "+instance.Spec.LightspeedStack.CaBundleSecretName))
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
		configVars[instance.Spec.LightspeedStack.CaBundleSecretName] = env.SetValue(caBundleHash)
	}

	// Validate optional Recipes ConfigMap
	if instance.Spec.Goose != nil && instance.Spec.Goose.Recipes != nil {
		_, recipesHash, err := configmap.GetConfigMapAndHashWithName(ctx, helper, *instance.Spec.Goose.Recipes, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					apiv1beta1.OpenStackAssistantReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					apiv1beta1.OpenStackAssistantRecipesWaitingMessage))
				return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
		configVars[*instance.Spec.Goose.Recipes] = env.SetValue(recipesHash)
	}

	// Validate optional Skills ConfigMap
	if instance.Spec.Goose != nil && instance.Spec.Goose.Skills != nil {
		_, skillsHash, err := configmap.GetConfigMapAndHashWithName(ctx, helper, *instance.Spec.Goose.Skills, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					apiv1beta1.OpenStackAssistantReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					apiv1beta1.OpenStackAssistantSkillsWaitingMessage))
				return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
		configVars[*instance.Spec.Goose.Skills] = env.SetValue(skillsHash)
	}

	// Validate optional Hints ConfigMap
	if instance.Spec.Goose != nil && instance.Spec.Goose.Hints != nil {
		_, hintsHash, err := configmap.GetConfigMapAndHashWithName(ctx, helper, *instance.Spec.Goose.Hints, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					apiv1beta1.OpenStackAssistantReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					apiv1beta1.OpenStackAssistantHintsWaitingMessage))
				return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
			}
			return ctrl.Result{}, err
		}
		configVars[*instance.Spec.Goose.Hints] = env.SetValue(hintsHash)
	}

	// Resolve MCP servers (auto-discover from OpenStackClientRef or use manual URL)
	resolvedMCPServers := make(map[string]string)
	mcpCaBundleSecretName := ""
	if instance.Spec.Goose != nil {
		for _, mcp := range instance.Spec.Goose.MCPServers {
			if mcp.OpenStackClientRef != "" {
				osclient := &uns.Unstructured{}
				osclient.SetGroupVersionKind(openStackClientGVK)
				err := r.Get(ctx, types.NamespacedName{
					Name:      mcp.OpenStackClientRef,
					Namespace: instance.Namespace,
				}, osclient)
				if err != nil {
					if k8s_errors.IsNotFound(err) {
						instance.Status.Conditions.Set(condition.FalseCondition(
							apiv1beta1.OpenStackAssistantReadyCondition,
							condition.RequestedReason,
							condition.SeverityInfo,
							"Waiting for OpenStackClient %s", mcp.OpenStackClientRef))
						return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
					}
					return ctrl.Result{}, fmt.Errorf("error looking up OpenStackClient %s: %w", mcp.OpenStackClientRef, err)
				}

				mcpEnabled, _, _ := uns.NestedBool(osclient.Object, "spec", "mcp", "enabled")
				if !mcpEnabled {
					instance.Status.Conditions.Set(condition.FalseCondition(
						apiv1beta1.OpenStackAssistantReadyCondition,
						condition.ErrorReason,
						condition.SeverityWarning,
						apiv1beta1.OpenStackAssistantReadyErrorMessage,
						"OpenStackClient "+mcp.OpenStackClientRef+" does not have MCP enabled"))
					return ctrl.Result{}, nil
				}

				caBundleSecretName, _, _ := uns.NestedString(osclient.Object, "spec", "caBundleSecretName")

				mcpSvcName := mcp.OpenStackClientRef + "-mcp"
				scheme := "http"
				if caBundleSecretName != "" {
					mcpCaBundleSecretName = caBundleSecretName
					scheme = "https"
				}
				mcpURL := fmt.Sprintf("%s://%s.%s.svc:8080/openstack/", scheme, mcpSvcName, instance.Namespace)
				resolvedMCPServers[mcp.Name] = mcpURL
				Log.Info("Auto-resolved MCP server", "name", mcp.Name, "url", mcpURL, "openstackClientRef", mcp.OpenStackClientRef)
			} else if mcp.URL != "" {
				resolvedMCPServers[mcp.Name] = mcp.URL
			}
		}
	}

	// Validate MCP CA bundle secret if auto-discovered
	if mcpCaBundleSecretName != "" && mcpCaBundleSecretName != instance.Spec.LightspeedStack.CaBundleSecretName {
		_, mcpCaHash, err := secret.GetSecret(ctx, helper, mcpCaBundleSecretName, instance.Namespace)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					apiv1beta1.OpenStackAssistantReadyCondition,
					condition.ErrorReason,
					condition.SeverityWarning,
					apiv1beta1.OpenStackAssistantReadyErrorMessage,
					"MCP CA bundle secret "+mcpCaBundleSecretName+" not found"))
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
		configVars["mcp-ca-bundle"] = env.SetValue(mcpCaHash)
	}

	// Build combined CA bundle when MCP TLS is in use, merging the internal CA
	// (tls-ca-bundle.pem) with the lightspeed CA (ca-bundle.crt) if present.
	// This handles same-secret, different-secret, and MCP-only cases.
	hasCombinedCA := false
	combinedCAPEM := ""
	if mcpCaBundleSecretName != "" {
		var lightspeedCA, mcpCA string

		if instance.Spec.LightspeedStack.CaBundleSecretName != "" {
			lightspeedCASecret, _, err := secret.GetSecret(ctx, helper, instance.Spec.LightspeedStack.CaBundleSecretName, instance.Namespace)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("error reading lightspeed CA secret: %w", err)
			}
			lightspeedCA = string(lightspeedCASecret.Data["ca-bundle.crt"])

			if mcpCaBundleSecretName == instance.Spec.LightspeedStack.CaBundleSecretName {
				mcpCA = string(lightspeedCASecret.Data["tls-ca-bundle.pem"])
			}
		}

		if mcpCA == "" {
			mcpCASecret, _, err := secret.GetSecret(ctx, helper, mcpCaBundleSecretName, instance.Namespace)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("error reading MCP CA secret: %w", err)
			}
			mcpCA = string(mcpCASecret.Data["tls-ca-bundle.pem"])
		}

		if mcpCA != "" {
			combinedCAPEM = mcpCA
			if lightspeedCA != "" {
				combinedCAPEM = lightspeedCA + "\n" + mcpCA
			}
			hasCombinedCA = true
		}
	}

	// Create/update entrypoint ConfigMap
	entrypointCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-entrypoint",
			Namespace: instance.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, r.Client, entrypointCM, func() error {
		entrypointCM.Data = map[string]string{
			"entrypoint.sh": assistant.EntrypointScript(),
		}
		if hasCombinedCA {
			entrypointCM.Data["combined-ca.crt"] = combinedCAPEM
		}
		return controllerutil.SetControllerReference(instance, entrypointCM, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating entrypoint ConfigMap: %w", err)
	}

	// Compute composite config hash
	configVarsHash, err := util.HashOfInputHashes(configVars)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Build PodSpec
	spec := assistant.AssistantPodSpec(instance, configVarsHash, resolvedMCPServers, hasCombinedCA)

	podSpecHash, err := util.ObjectHash(spec)
	if err != nil {
		return ctrl.Result{}, err
	}

	podSpecHashName := "podSpec"

	// Create/update Pod
	assistantPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, assistantPod, func() error {
		isPodUpdate := !assistantPod.CreationTimestamp.IsZero()
		currentPodSpecHash := instance.Status.Hash[podSpecHashName]
		podServiceAccountDrifted := assistantPod.Spec.ServiceAccountName != spec.ServiceAccountName
		if !isPodUpdate || currentPodSpecHash != podSpecHash || podServiceAccountDrifted {
			assistantPod.Spec = spec
		}
		assistantPod.Labels = util.MergeStringMaps(assistantPod.Labels, assistantLabels)

		return controllerutil.SetControllerReference(instance, assistantPod, r.Scheme)
	})
	if err != nil {
		var forbiddenPodSpecChangeErr *k8s_errors.StatusError

		forbiddenPodSpec := false
		if errors.As(err, &forbiddenPodSpecChangeErr) {
			if forbiddenPodSpecChangeErr.ErrStatus.Reason == metav1.StatusReasonForbidden {
				forbiddenPodSpec = true
			}
		}

		if forbiddenPodSpec || k8s_errors.IsInvalid(err) {
			if err := r.Delete(ctx, assistantPod); err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("error deleting OpenStackAssistant pod %s: %w", assistantPod.Name, err)
			}
			Log.Info(fmt.Sprintf("OpenStackAssistant pod deleted due to change %s", err.Error()))

			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to create or update pod %s: %w", assistantPod.Name, err)
	}

	instance.Status.Hash, _ = util.SetHash(instance.Status.Hash, podSpecHashName, podSpecHash)
	instance.Status.PodName = assistantPod.Name

	if op != controllerutil.OperationResultNone {
		util.LogForObject(
			helper,
			fmt.Sprintf("Pod %s successfully reconciled - operation: %s", assistantPod.Name, string(op)),
			instance,
		)
	}

	// Force-delete pods stuck in Terminating >3 minutes
	if assistantPod.DeletionTimestamp != nil {
		terminatingDuration := time.Since(assistantPod.DeletionTimestamp.Time)
		if terminatingDuration > time.Minute*3 {
			err := r.Delete(ctx, assistantPod, client.GracePeriodSeconds(0))
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to force delete pod: %w", err)
			}
		}
	}

	// Check pod readiness
	podReady := false
	for _, cond := range assistantPod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			podReady = true
			break
		}
	}

	if podReady {
		instance.Status.Conditions.MarkTrue(
			apiv1beta1.OpenStackAssistantReadyCondition,
			apiv1beta1.OpenStackAssistantReadyMessage,
		)
	} else {
		instance.Status.Conditions.Set(condition.FalseCondition(
			apiv1beta1.OpenStackAssistantReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			apiv1beta1.OpenStackAssistantReadyRunningMessage))
	}

	return ctrl.Result{}, nil
}

// reconcileGooseServiceAccount ensures the ServiceAccount that the assistant/
// goose pod runs as exists, is bound to the nonroot-v2 SecurityContextConstraint,
// and is granted GET /ls-access so Lightspeed's k8s auth module will accept its
// SA token. Unlike the ServiceAccount openstack-operator creates for the MCP
// sidecar (apiv1beta1.OpenStackAssistantServiceAccountName), this SA carries no
// k8s API resource RBAC grants.
func (r *OpenStackAssistantReconciler) reconcileGooseServiceAccount(
	ctx context.Context,
	helper *helper.Helper,
	instance *apiv1beta1.OpenStackAssistant,
) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiv1beta1.OpenStackAssistantGooseServiceAccountName,
			Namespace: instance.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrPatch(ctx, r.Client, sa, func() error {
		return controllerutil.SetControllerReference(instance, sa, r.Scheme)
	}); err != nil {
		return fmt.Errorf("error creating goose ServiceAccount: %w", err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiv1beta1.OpenStackAssistantGooseServiceAccountName + "-" + apiv1beta1.OpenStackAssistantGooseSCCName,
			Namespace: instance.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrPatch(ctx, r.Client, rb, func() error {
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "system:openshift:scc:" + apiv1beta1.OpenStackAssistantGooseSCCName,
		}
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      sa.Name,
				Namespace: instance.Namespace,
			},
		}
		return controllerutil.SetControllerReference(instance, rb, r.Scheme)
	}); err != nil {
		return fmt.Errorf("error creating goose SCC RoleBinding: %w", err)
	}

	// ClusterRole/ClusterRoleBinding are cluster-scoped and cannot be owned by
	// the namespaced OpenStackAssistant CR, so they are cleaned up explicitly
	// in reconcileDelete.
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: apiv1beta1.OpenStackAssistantGooseLSAccessClusterRoleName(instance.Namespace),
		},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, r.Client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				NonResourceURLs: []string{apiv1beta1.OpenStackAssistantGooseLSAccessPath},
				Verbs:           []string{"get"},
			},
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error creating goose /ls-access ClusterRole: %w", err)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: apiv1beta1.OpenStackAssistantGooseLSAccessClusterRoleBindingName(instance.Namespace),
		},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, r.Client, crb, func() error {
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     role.Name,
		}
		crb.Subjects = []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      sa.Name,
				Namespace: instance.Namespace,
			},
		}
		return nil
	}); err != nil {
		return fmt.Errorf("error creating goose /ls-access ClusterRoleBinding: %w", err)
	}

	util.LogForObject(helper, "Goose ServiceAccount, SCC RoleBinding, and /ls-access RBAC reconciled", instance)

	return nil
}

// reconcileDelete removes cluster-scoped resources owned by this
// OpenStackAssistant that cannot be garbage-collected via owner references,
// then drops the finalizer so the CR can finish deleting.
func (r *OpenStackAssistantReconciler) reconcileDelete(
	ctx context.Context,
	helper *helper.Helper,
	instance *apiv1beta1.OpenStackAssistant,
) error {
	Log := r.GetLogger(ctx)
	Log.Info("OpenStackAssistant Reconciling Delete")

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: apiv1beta1.OpenStackAssistantGooseLSAccessClusterRoleBindingName(instance.Namespace),
		},
	}
	if err := client.IgnoreNotFound(r.Client.Delete(ctx, crb)); err != nil {
		return fmt.Errorf("error deleting goose /ls-access ClusterRoleBinding: %w", err)
	}

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: apiv1beta1.OpenStackAssistantGooseLSAccessClusterRoleName(instance.Namespace),
		},
	}
	if err := client.IgnoreNotFound(r.Client.Delete(ctx, role)); err != nil {
		return fmt.Errorf("error deleting goose /ls-access ClusterRole: %w", err)
	}

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("OpenStackAssistant Reconciling Delete completed")
	return nil
}

// fields to index to reconcile when change
const (
	providerSecretField = ".spec.lightspeedStack.providerSecret"
	caBundleSecretField = ".spec.lightspeedStack.caBundleSecretName"
	recipesField        = ".spec.goose.recipes"
	skillsField         = ".spec.goose.skills"
	hintsField          = ".spec.goose.hints"
)

var allWatchFields = []string{
	providerSecretField,
	caBundleSecretField,
	recipesField,
	skillsField,
	hintsField,
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenStackAssistantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctx := context.Background()

	if err := mgr.GetFieldIndexer().IndexField(ctx, &apiv1beta1.OpenStackAssistant{}, providerSecretField, func(rawObj client.Object) []string {
		cr := rawObj.(*apiv1beta1.OpenStackAssistant)
		if cr.Spec.LightspeedStack.ProviderSecret == "" {
			return nil
		}
		return []string{cr.Spec.LightspeedStack.ProviderSecret}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &apiv1beta1.OpenStackAssistant{}, caBundleSecretField, func(rawObj client.Object) []string {
		cr := rawObj.(*apiv1beta1.OpenStackAssistant)
		if cr.Spec.LightspeedStack.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.LightspeedStack.CaBundleSecretName}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &apiv1beta1.OpenStackAssistant{}, recipesField, func(rawObj client.Object) []string {
		cr := rawObj.(*apiv1beta1.OpenStackAssistant)
		if cr.Spec.Goose == nil || cr.Spec.Goose.Recipes == nil || *cr.Spec.Goose.Recipes == "" {
			return nil
		}
		return []string{*cr.Spec.Goose.Recipes}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &apiv1beta1.OpenStackAssistant{}, skillsField, func(rawObj client.Object) []string {
		cr := rawObj.(*apiv1beta1.OpenStackAssistant)
		if cr.Spec.Goose == nil || cr.Spec.Goose.Skills == nil || *cr.Spec.Goose.Skills == "" {
			return nil
		}
		return []string{*cr.Spec.Goose.Skills}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &apiv1beta1.OpenStackAssistant{}, hintsField, func(rawObj client.Object) []string {
		cr := rawObj.(*apiv1beta1.OpenStackAssistant)
		if cr.Spec.Goose == nil || cr.Spec.Goose.Hints == nil || *cr.Spec.Goose.Hints == "" {
			return nil
		}
		return []string{*cr.Spec.Goose.Hints}
	}); err != nil {
		return err
	}

	openStackClientWatch := &uns.Unstructured{}
	openStackClientWatch.SetGroupVersionKind(openStackClientGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1beta1.OpenStackAssistant{}).
		Named("openstackassistant").
		Owns(&corev1.Pod{}).
		Owns(&corev1.ConfigMap{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			openStackClientWatch,
			handler.EnqueueRequestsFromMapFunc(r.findAssistantsForOpenStackClient),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *OpenStackAssistantReconciler) findAssistantsForOpenStackClient(ctx context.Context, src client.Object) []reconcile.Request {
	Log := r.GetLogger(ctx)
	requests := []reconcile.Request{}

	crList := &apiv1beta1.OpenStackAssistantList{}
	if err := r.List(ctx, crList, client.InNamespace(src.GetNamespace())); err != nil {
		Log.Error(err, "listing OpenStackAssistants for OpenStackClient change")
		return requests
	}

	for _, item := range crList.Items {
		if item.Spec.Goose == nil {
			continue
		}
		for _, mcp := range item.Spec.Goose.MCPServers {
			if mcp.OpenStackClientRef == src.GetName() {
				Log.Info("OpenStackClient changed, reconciling assistant",
					"openstackClient", src.GetName(),
					"assistant", item.GetName())
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				})
				break
			}
		}
	}

	return requests
}

func (r *OpenStackAssistantReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(context.Background())

	for _, field := range allWatchFields {
		crList := &apiv1beta1.OpenStackAssistantList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}
