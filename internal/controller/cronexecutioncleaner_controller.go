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
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	lifecyclev1alpha1 "github.com/bhatpriyanka8/cron-execution-cleaner/api/v1alpha1"
)

// CronExecutionCleanerReconciler reconciles a CronExecutionCleaner object
type CronExecutionCleanerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// RBAC permissions
//+kubebuilder:rbac:groups=lifecycle.github.io,resources=cronexecutioncleaners,verbs=get;list;watch
//+kubebuilder:rbac:groups=lifecycle.github.io,resources=cronexecutioncleaners/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=lifecycle.github.io,resources=cronexecutioncleaners/finalizers,verbs=update

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;delete

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CronExecutionCleaner object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *CronExecutionCleanerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling CronExecutionCleaner", "name", req.NamespacedName)

	var cleaner lifecyclev1alpha1.CronExecutionCleaner
	if err := r.Get(ctx, req.NamespacedName, &cleaner); err != nil {
		log.Error(err, "unable to fetch CronExecutionCleaner")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := validateSpec(ctx, &cleaner); err != nil {
		log.Error(err, "Invalid CronExecutionCleaner spec, skipping reconciliation", "name", req.NamespacedName)
		// record event
		r.Recorder.Event(
			&cleaner,
			corev1.EventTypeWarning,
			"InvalidSpec",
			err.Error(),
		)

		setCondition(
			&cleaner,
			"Ready",
			metav1.ConditionFalse,
			"InvalidSpec",
			err.Error(),
		)

		_ = r.Status().Update(ctx, &cleaner)
		return ctrl.Result{}, nil
	}

	log.Info(
		"Loaded CronExecutionCleaner spec",
		"Namespace", cleaner.Spec.Namespace,
		"CronJobName", cleaner.Spec.CronJobName,
		"Retain", cleaner.Spec.Retain,
		"CleanupStuck", cleaner.Spec.CleanupStuck,
		"RunInterval", cleaner.Spec.RunInterval,
	)

	var jobList batchv1.JobList

	err := r.List(ctx, &jobList, client.InNamespace(cleaner.Spec.Namespace))
	if err != nil {
		log.Error(err, "unable to list Jobs for CronExecutionCleaner")
		return ctrl.Result{}, err
	}

	ownedJobs := filterJobsByOwner(jobList.Items, cleaner.Spec.CronJobName)
	log.Info(
		"Found Jobs owned by CronJob",
		"cronJob", cleaner.Spec.CronJobName,
		"count", len(ownedJobs),
	)

	activeJobs, succeededJobs, failedJobs := classifyJobs(ownedJobs)
	log.Info(
		"Job classification",
		"active", len(activeJobs),
		"succeeded", len(succeededJobs),
		"failed", len(failedJobs),
	)

	stuckJobs := []batchv1.Job{}
	deletedCount := 0

	if cleaner.Spec.CleanupStuck.Enabled {
		now := time.Now()
		stuckAfter := cleaner.Spec.CleanupStuck.StuckAfter.Duration
		stuckJobs = detectStuckJobs(activeJobs, stuckAfter, now)

		log.Info(
			"Stuck job detection",
			"enabled", true,
			"stuckAfter", stuckAfter.String(),
			"count", len(stuckJobs),
		)
		deletedCount += r.deleteJobs(ctx, stuckJobs, "stuck")

		// Retention logic for succeeded jobs
		excessSucceeded := excessJobs(succeededJobs, cleaner.Spec.Retain.SuccessfulJobs)

		log.Info(
			"Succeeded job retention evaluation",
			"retain", cleaner.Spec.Retain.SuccessfulJobs,
			"total", len(succeededJobs),
			"excess", len(excessSucceeded),
		)
		deletedCount += r.deleteJobs(ctx, excessSucceeded, "succeeded")

		// Retention logic for failed jobs
		excessFailed := excessJobs(failedJobs, cleaner.Spec.Retain.FailedJobs)

		log.Info(
			"Failed job retention evaluation",
			"retain", cleaner.Spec.Retain.FailedJobs,
			"total", len(failedJobs),
			"excess", len(excessFailed),
		)
		deletedCount += r.deleteJobs(ctx, excessFailed, "failed")

		if deletedCount > 0 {
			now := metav1.Now()

			cleaner.Status.LastRunTime = &now
			cleaner.Status.JobsDeleted += deletedCount
			cleaner.Status.PodsDeleted += deletedCount // 1 pod per job in our setup

			if err := r.Status().Update(ctx, &cleaner); err != nil {
				log.Error(err, "Failed to update CronExecutionCleaner status")
				return ctrl.Result{}, err
			}
		}
		log.Info("Cleanup summary", "totalDeleted", deletedCount)
	}
	setCondition(
		&cleaner,
		"Ready",
		metav1.ConditionTrue,
		"ReconcileSuccess",
		"Cleanup executed successfully",
	)

	_ = r.Status().Update(ctx, &cleaner)

	return ctrl.Result{
		RequeueAfter: cleaner.Spec.RunInterval.Duration,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CronExecutionCleanerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("cronexecutioncleaner")
	return ctrl.NewControllerManagedBy(mgr).
		For(&lifecyclev1alpha1.CronExecutionCleaner{}).
		Complete(r)
}
