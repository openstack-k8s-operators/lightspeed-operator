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

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcilePostgresResources reconciles Postgres prerequisite resources (Phase 1):
// ConfigMap, Bootstrap Secret, Password Secret, and Network Policy.
// Uses continue-on-error pattern to attempt all resources even if some fail.
func ReconcilePostgresResources(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	tasks := []ReconcileTask{
		{Name: "PostgresConfigMap", Task: reconcilePostgresConfigMap},
		{Name: "PostgresBootstrapSecret", Task: reconcilePostgresBootstrapSecret},
		{Name: "PostgresSecret", Task: reconcilePostgresSecret},
		{Name: "PostgresNetworkPolicy", Task: reconcilePostgresNetworkPolicy},
	}

	return ReconcileTasks(h, ctx, instance, tasks)
}

// ReconcilePostgresDeployment reconciles the Postgres Deployment and Service (Phase 2).
// Uses fail-fast pattern where the first error stops execution.
func ReconcilePostgresDeployment(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	tasks := []ReconcileTask{
		{Name: "PostgresPVC", Task: reconcilePostgresPVC},
		{Name: "PostgresDeployment", Task: reconcilePostgresDeploymentTask},
		{Name: "PostgresService", Task: reconcilePostgresServiceTask},
	}

	return ReconcileTasksFailFast(h, ctx, instance, tasks)
}

func reconcilePostgresConfigMap(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresConfigMapName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), cm, func() error {
		// Set static postgres configuration
		cm.Data = map[string]string{
			PostgresConfigKey: PostgresConfigMapContent,
		}
		// Set owner reference
		return controllerutil.SetControllerReference(h.GetBeforeObject(), cm, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresConfigMap, err)
	}

	h.GetLogger().Info("Postgres ConfigMap reconciled", "name", cm.Name, "result", result)
	return nil
}

func reconcilePostgresBootstrapSecret(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresBootstrapSecretName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), secret, func() error {
		// Set bootstrap script data
		secret.StringData = map[string]string{
			PostgresBootstrapScript:    PostgresBootStrapScriptContent,
			PostgresBootstrapSQLScript: PostgresBootStrapSQLContent,
		}
		// Set owner reference
		return controllerutil.SetControllerReference(h.GetBeforeObject(), secret, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresBootstrapSecret, err)
	}

	h.GetLogger().Info("Postgres bootstrap secret reconciled", "name", secret.Name, "result", result)
	return nil
}

func reconcilePostgresSecret(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	// Check if secret exists - if not, cleanup old secrets first
	checkSecret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      PostgresSecretName,
		Namespace: h.GetBeforeObject().GetNamespace(),
	}
	err := h.GetClient().Get(ctx, secretKey, checkSecret)
	if errors.IsNotFound(err) {
		// Delete any old postgres secrets before creating a new one
		if err := deleteOldPostgresSecrets(h, ctx); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("%w: %v", ErrGetPostgresSecret, err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresSecretName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}

		// Only set password if not already present (preserve existing password)
		if secret.Data[OpenStackLightspeedComponentPasswordFileName] == nil {
			const PostgreSQLPasswordLen = 32
			password, err := generateRandomString(PostgreSQLPasswordLen)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrGeneratePostgresSecret, err)
			}

			secret.Data[OpenStackLightspeedComponentPasswordFileName] = []byte(password)
		}

		secret.Data[PostgresUsernameSecretKey] = []byte(PostgresSQLUsername)
		return controllerutil.SetControllerReference(h.GetBeforeObject(), secret, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresSecret, err)
	}

	h.GetLogger().Info("Postgres secret reconciled", "name", secret.Name, "result", result)
	return nil
}

func deleteOldPostgresSecrets(h *common_helper.Helper, ctx context.Context) error {
	labelSelector := labels.Set{"app.kubernetes.io/name": "lightspeed-service-postgres"}.AsSelector()
	matchingLabels := client.MatchingLabelsSelector{Selector: labelSelector}
	deleteOptions := &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			Namespace:     h.GetBeforeObject().GetNamespace(),
			LabelSelector: matchingLabels,
		},
	}
	if err := h.GetClient().DeleteAllOf(ctx, &corev1.Secret{}, deleteOptions); err != nil {
		return fmt.Errorf("failed to delete old Postgres secrets: %w", err)
	}
	return nil
}

func reconcilePostgresNetworkPolicy(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresNetworkPolicyName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), np, func() error {
		// Set Spec (wholesale replacement, same as before)
		// Restricts ingress to Postgres to only allow traffic from app server pods
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: generatePostgresSelectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: generateAppServerSelectorLabels(),
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: toPtr(corev1.ProtocolTCP),
							Port:     toPtr(intstr.FromInt32(PostgresServicePort)),
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		}
		// Set owner reference
		return controllerutil.SetControllerReference(h.GetBeforeObject(), np, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresNetworkPolicy, err)
	}

	h.GetLogger().Info("Postgres NetworkPolicy reconciled", "name", np.Name, "result", result)
	return nil
}

func reconcilePostgresPVC(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	requestedQty := resource.MustParse(PostgresDataPVCDefaultSize)
	var storageClass string
	if instance.Spec.Database != nil {
		if !instance.Spec.Database.Size.IsZero() {
			requestedQty = instance.Spec.Database.Size
		}
		storageClass = instance.Spec.Database.Class
	}

	pvc := &corev1.PersistentVolumeClaim{}
	pvcKey := client.ObjectKey{
		Name:      PostgresDataPVCName,
		Namespace: h.GetBeforeObject().GetNamespace(),
	}

	err := h.GetClient().Get(ctx, pvcKey, pvc)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("%w: %v", ErrGetPostgresPVC, err)
	}

	if err == nil {
		// PVC already exists, validate if the size matches
		existingQty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if requestedQty.Cmp(existingQty) != 0 {
			return fmt.Errorf("%w: requested size %s but existing PVC has %s",
				ErrPostgresPVCSizeMismatch, requestedQty.String(), existingQty.String())
		}
		h.GetLogger().Info("Reusing the existing PostgreSQL PVC with a matching size", "name", pvc.Name)
		return nil
	}

	// PVC does not exist, create it
	pvc = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresDataPVCName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), pvc, func() error {
		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		pvc.Spec.Resources = corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: requestedQty,
			},
		}
		if storageClass != "" {
			pvc.Spec.StorageClassName = &storageClass
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresPVC, err)
	}

	h.GetLogger().Info("Postgres PVC reconciled", "name", pvc.Name, "result", result)
	return nil
}

func reconcilePostgresDeploymentTask(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresDeploymentName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), deployment, func() error {
		currentConfigMapVersion, err := getConfigMapResourceVersion(ctx, h, PostgresConfigMapName, h.GetBeforeObject().GetNamespace())
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("%w: %v", ErrGetPostgresConfigMap, err)
		}

		currentSecretVersion, err := getSecretResourceVersion(ctx, h, PostgresSecretName, h.GetBeforeObject().GetNamespace())
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("%w: %v", ErrGetPostgresSecret, err)
		}

		// Build the desired deployment pod spec
		podTemplateSpec := buildPostgresPodTemplateSpec()

		// Initialize annotations map if needed
		if podTemplateSpec.Annotations == nil {
			podTemplateSpec.Annotations = map[string]string{}
		}

		// Store the current ConfigMap and Secret versions in pod template annotations.
		// When either changes, Kubernetes will see a pod template change and trigger a rollout.
		podTemplateSpec.Annotations[PostgresConfigMapResourceVersionAnnotation] = currentConfigMapVersion
		podTemplateSpec.Annotations[PostgresSecretResourceVersionAnnotation] = currentSecretVersion

		// Selective field updates (avoid update loops)
		replicas := int32(1)
		deployment.Spec.Replicas = &replicas
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: generatePostgresSelectorLabels(),
		}
		deployment.Spec.Template = podTemplateSpec

		// Also set RevisionHistoryLimit to match current behavior
		revisionHistoryLimit := int32(1)
		deployment.Spec.RevisionHistoryLimit = &revisionHistoryLimit

		// Set owner reference
		return controllerutil.SetControllerReference(h.GetBeforeObject(), deployment, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresDeployment, err)
	}

	h.GetLogger().Info("Postgres Deployment reconciled", "name", deployment.Name, "result", result)
	return nil
}

func reconcilePostgresServiceTask(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresServiceName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), svc, func() error {
		// Selective field updates (preserves ClusterIP, ClusterIPs, etc.)
		svc.Spec.Selector = generatePostgresSelectorLabels()
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Port:       PostgresServicePort,
				Protocol:   corev1.ProtocolTCP,
				Name:       "server",
				TargetPort: intstr.Parse("server"),
			},
		}
		svc.Spec.Type = corev1.ServiceTypeClusterIP

		// Set service-ca annotation for TLS certificate provisioning
		if svc.Annotations == nil {
			svc.Annotations = make(map[string]string)
		}
		svc.Annotations[ServingCertSecretAnnotationKey] = PostgresCertsSecretName

		// Set owner reference
		return controllerutil.SetControllerReference(h.GetBeforeObject(), svc, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreatePostgresService, err)
	}

	h.GetLogger().Info("Postgres Service reconciled", "name", svc.Name, "result", result)
	return nil
}
