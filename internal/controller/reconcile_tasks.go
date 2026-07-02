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
)

// ReconcileFunc is a function that reconciles a single lcore resource.
type ReconcileFunc func(*common_helper.Helper, context.Context, *apiv1beta1.OpenStackLightspeed) error

// ReconcileTask pairs a task name with its reconcile function.
type ReconcileTask struct {
	Name string
	Task ReconcileFunc
}

// ReconcileTasks executes a list of reconciliation tasks sequentially, logging
// each failure but continuing through the remaining tasks. It returns the first
// error encountered, wrapped with the failing task's name.
func ReconcileTasks(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed, tasks []ReconcileTask) error {
	logger := h.GetLogger()
	logger.Info("reconciling resources")

	var firstErr error
	for _, t := range tasks {
		if err := t.Task(h, ctx, instance); err != nil {
			logger.Error(err, "failed to reconcile resource", "task", t.Name)
			if firstErr == nil {
				firstErr = fmt.Errorf("task %s: %w", t.Name, err)
			}
		}
	}

	if firstErr != nil {
		return firstErr
	}

	logger.Info("resources reconciled")
	return nil
}

// ReconcileTasksFailFast executes a list of reconciliation tasks sequentially,
// stopping immediately at the first error encountered. This is useful for tasks
// that have strict ordering dependencies where subsequent tasks cannot proceed
// if earlier ones fail.
func ReconcileTasksFailFast(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed, tasks []ReconcileTask) error {
	for _, t := range tasks {
		if err := t.Task(h, ctx, instance); err != nil {
			return fmt.Errorf("task %s: %w", t.Name, err)
		}
	}
	return nil
}
