/*
Copyright 2026.

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
	"slices"
	"strconv"
	"strings"
	"time"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
)

// ReconcileConsoleResources reconciles Phase 1 console resources: ConfigMap (nginx),
// NetworkPolicy, and ServiceAccount. Uses a continue-on-error pattern.
func ReconcileConsoleResources(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	tasks := []ReconcileTask{
		{Name: "ConsoleConfigMap", Task: reconcileConsoleConfigMap},
		{Name: "ConsoleNetworkPolicy", Task: reconcileConsoleNetworkPolicy},
		{Name: "ConsoleServiceAccount", Task: reconcileConsoleServiceAccount},
	}

	return ReconcileTasks(h, ctx, instance, tasks)
}

// ReconcileConsoleDeployment reconciles Phase 2 console resources: Deployment, Service,
// TLS secret, ConsolePlugin, and console activation. Uses a fail-fast pattern.
func ReconcileConsoleDeployment(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	tasks := []ReconcileTask{
		{Name: "ConsoleDeployment", Task: reconcileConsoleDeploymentResource},
		{Name: "ConsoleService", Task: reconcileConsoleService},
		{Name: "ConsoleTLSSecret", Task: reconcileConsoleTLSSecret},
		{Name: "ConsolePlugin", Task: reconcileConsolePlugin},
		{Name: "ActivateConsole", Task: activateConsole},
	}

	return ReconcileTasksFailFast(h, ctx, instance, tasks)
}

// reconcileConsoleConfigMap ensures the console plugin nginx ConfigMap exists.
func reconcileConsoleConfigMap(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIConfigMapName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), cm, func() error {
		cm.Data = map[string]string{
			"nginx.conf": buildConsoleNginxConfig(),
		}
		return controllerutil.SetControllerReference(h.GetBeforeObject(), cm, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsoleConfigMap, err)
	}

	logger.Info("Console ConfigMap reconciled", "name", cm.Name, "result", result)
	return nil
}

// reconcileConsoleNetworkPolicy ensures the console plugin network policy exists.
func reconcileConsoleNetworkPolicy(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUINetworkPolicyName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), np, func() error {
		np.Spec = buildConsoleNetworkPolicySpec()
		return controllerutil.SetControllerReference(h.GetBeforeObject(), np, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsoleNetPolicy, err)
	}

	logger.Info("Console NetworkPolicy reconciled", "name", np.Name, "result", result)
	return nil
}

// reconcileConsoleServiceAccount ensures the console plugin service account exists.
func reconcileConsoleServiceAccount(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIServiceAccountName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), sa, func() error {
		return controllerutil.SetControllerReference(h.GetBeforeObject(), sa, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsoleSA, err)
	}

	logger.Info("Console ServiceAccount reconciled", "name", sa.Name, "result", result)
	return nil
}

// consoleImageForVersion selects the console image based on detected OCP version.
// OCP < 4.19 or failed to get cluster version uses PatternFly 5
// OCP >= 4.19 uses PatternFly 6.
func consoleImageForVersion(version string) string {
	if version == "" {
		return apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL
	}
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		major, err1 := strconv.Atoi(parts[0])
		minor, err2 := strconv.Atoi(parts[1])
		// OCP < 4 can't run this operator, so major != 4 effectively means OCP 5+
		if err1 == nil && err2 == nil && (major != 4 || minor >= 19) {
			return apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImageURL
		}
	}
	return apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImagePF5URL
}

// resolveConsoleImage selects the console plugin image based on OCP cluster version.
func resolveConsoleImage(ctx context.Context, h *common_helper.Helper) string {
	logger := h.GetLogger()

	version, err := DetectOCPVersion(ctx, h)
	if err != nil {
		logger.Info("Failed to detect OCP version for console image, using default", "error", err)
	}

	image := consoleImageForVersion(version)

	if image == apiv1beta1.OpenStackLightspeedDefaultValues.ConsoleImageURL {
		logger.Info("OCP >= 4.19, using PatternFly 6 console image", "version", version)
	} else {
		logger.Info("Using PatternFly 5 console image", "version", version)
	}

	return image
}

// reconcileConsoleDeploymentResource ensures the console plugin deployment exists.
func reconcileConsoleDeploymentResource(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	consoleImage := resolveConsoleImage(ctx, h)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIDeploymentName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), deployment, func() error {
		spec := buildConsoleDeploymentSpec(consoleImage, instance)
		deployment.Spec.Replicas = spec.Replicas
		deployment.Spec.Selector = spec.Selector
		deployment.Spec.Template = spec.Template
		return controllerutil.SetControllerReference(h.GetBeforeObject(), deployment, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsoleDeployment, err)
	}

	logger.Info("Console Deployment reconciled", "name", deployment.Name, "result", result)
	return nil
}

// reconcileConsoleService ensures the console plugin service exists.
func reconcileConsoleService(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIServiceName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), svc, func() error {
		svc.Spec.Selector = generateConsoleSelectorLabels()
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Port:       ConsoleUIHTTPSPort,
				Name:       "https",
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString("https"),
			},
		}
		svc.Spec.Type = corev1.ServiceTypeClusterIP

		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations[ServingCertSecretAnnotationKey] = ConsoleUIServiceCertSecretName

		return controllerutil.SetControllerReference(h.GetBeforeObject(), svc, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsoleService, err)
	}

	logger.Info("Console Service reconciled", "name", svc.Name, "result", result)
	return nil
}

// reconcileConsoleTLSSecret waits for the console TLS secret to be populated by
// the service-ca operator.
func reconcileConsoleTLSSecret(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()
	logger.Info("waiting for console TLS secret", "name", ConsoleUIServiceCertSecretName)

	secretKey := client.ObjectKey{
		Name:      ConsoleUIServiceCertSecretName,
		Namespace: h.GetBeforeObject().GetNamespace(),
	}

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		secret := &corev1.Secret{}
		if err := h.GetClient().Get(ctx, secretKey, secret); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		_, hasKey := secret.Data["tls.key"]
		_, hasCert := secret.Data["tls.crt"]
		return hasKey && hasCert, nil
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsoleTLSSecret, err)
	}

	logger.Info("Console TLS secret is ready", "name", ConsoleUIServiceCertSecretName)
	return nil
}

// reconcileConsolePlugin ensures the ConsolePlugin CR exists.
func reconcileConsolePlugin(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()
	namespace := h.GetBeforeObject().GetNamespace()

	plugin := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name: ConsoleUIPluginName,
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), plugin, func() error {
		plugin.Spec = buildConsolePluginSpec(namespace)
		// ConsolePlugin is cluster-scoped, no owner reference
		return nil
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrReconcileConsolePlugin, err)
	}

	logger.Info("ConsolePlugin reconciled", "name", plugin.Name, "result", result)
	return nil
}

// activateConsole adds the console plugin to the Console CR's plugin list.
func activateConsole(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		console := &openshiftv1.Console{}
		err := h.GetClient().Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Console CR not found, skipping plugin activation")
				return nil
			}
			return fmt.Errorf("failed to get Console CR: %w", err)
		}

		if console.Spec.Plugins == nil {
			console.Spec.Plugins = []string{ConsoleUIPluginName}
		} else if !slices.Contains(console.Spec.Plugins, ConsoleUIPluginName) {
			console.Spec.Plugins = append(console.Spec.Plugins, ConsoleUIPluginName)
		} else {
			return nil
		}

		return h.GetClient().Update(ctx, console)
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrActivateConsolePlugin, err)
	}

	logger.Info("Console plugin activated")
	return nil
}

// reconcileDeleteConsole deactivates the console plugin and deletes the ConsolePlugin CR.
func reconcileDeleteConsole(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	// Deactivate: remove plugin from Console CR
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		console := &openshiftv1.Console{}
		err := h.GetClient().Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Console CR not found, skipping deactivation")
				return nil
			}
			return fmt.Errorf("%w: %v", ErrDeactivateConsolePlugin, err)
		}

		if console.Spec.Plugins == nil {
			return nil
		}
		if !slices.Contains(console.Spec.Plugins, ConsoleUIPluginName) {
			return nil
		}

		console.Spec.Plugins = slices.DeleteFunc(console.Spec.Plugins, func(name string) bool {
			return name == ConsoleUIPluginName
		})

		return h.GetClient().Update(ctx, console)
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeactivateConsolePlugin, err)
	}
	logger.Info("Console plugin deactivated")

	// Delete ConsolePlugin CR
	plugin := &consolev1.ConsolePlugin{}
	err = h.GetClient().Get(ctx, client.ObjectKey{Name: ConsoleUIPluginName}, plugin)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ConsolePlugin not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%w: %v", ErrDeleteConsolePlugin, err)
	}

	err = h.GetClient().Delete(ctx, plugin)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("%w: %v", ErrDeleteConsolePlugin, err)
	}

	logger.Info("ConsolePlugin deleted")
	return nil
}
