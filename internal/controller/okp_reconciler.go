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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ReconcileOKPDeployment reconciles the OKP Deployment and Service.
func ReconcileOKPDeployment(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	tasks := []ReconcileTask{
		{Name: "OKPDeployment", Task: reconcileOKPDeployment},
		{Name: "OKPService", Task: reconcileOKPService},
	}
	return ReconcileTasksFailFast(h, ctx, instance, tasks)
}

func reconcileOKPDeployment(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OKPDeploymentName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), deployment, func() error {
		podTemplateSpec := buildOKPPodTemplateSpec(instance)

		replicas := int32(1)
		deployment.Spec.Replicas = &replicas
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: generateOKPSelectorLabels(),
		}
		deployment.Spec.Template = podTemplateSpec

		return controllerutil.SetControllerReference(h.GetBeforeObject(), deployment, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateOKPDeployment, err)
	}

	logger.Info("OKP Deployment reconciled", "name", deployment.Name, "result", result)
	return nil
}

func reconcileOKPService(h *common_helper.Helper, ctx context.Context, _ *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OKPServiceName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), svc, func() error {
		svc.Spec.Selector = generateOKPSelectorLabels()
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       OKPServicePort,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString("okp"),
			},
		}
		svc.Spec.Type = corev1.ServiceTypeClusterIP

		return controllerutil.SetControllerReference(h.GetBeforeObject(), svc, h.GetScheme())
	})

	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateOKPService, err)
	}

	logger.Info("OKP Service reconciled", "name", svc.Name, "result", result)
	return nil
}

func buildOKPPodTemplateSpec(instance *apiv1beta1.OpenStackLightspeed) corev1.PodTemplateSpec {
	envVars := []corev1.EnvVar{}
	if instance.Spec.OKP != nil && instance.Spec.OKP.AccessKey != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instance.Spec.OKP.AccessKey,
					},
					Key: OKPAccessKeySecretKey,
				},
			},
		})
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: generateOKPSelectorLabels(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  OKPContainerName,
					Image: apiv1beta1.OpenStackLightspeedDefaultValues.OKPImageURL,
					Ports: []corev1.ContainerPort{{Name: "okp", ContainerPort: OKPContainerPort}},
					Env:   envVars,
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt32(OKPContainerPort),
							},
						},
						InitialDelaySeconds: 30,
						PeriodSeconds:       10,
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt32(OKPContainerPort),
							},
						},
						InitialDelaySeconds: 60,
						PeriodSeconds:       20,
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
	}
}
