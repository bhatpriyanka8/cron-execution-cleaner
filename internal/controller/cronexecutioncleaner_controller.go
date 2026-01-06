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
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	lifecyclev1alpha1 "github.com/bhatpriyanka8/cron-execution-cleaner/api/v1alpha1"
)

// CronExecutionCleanerReconciler reconciles a CronExecutionCleaner object
type CronExecutionCleanerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=lifecycle.github.io,resources=cronexecutioncleaners,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=lifecycle.github.io,resources=cronexecutioncleaners/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=lifecycle.github.io,resources=cronexecutioncleaners/finalizers,verbs=update

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

	// TODO(user): your logic here
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling CronExecutionCleaner", "name", req.NamespacedName)

	var cleaner lifecyclev1alpha1.CronExecutionCleaner
	if err := r.Get(ctx, req.NamespacedName, &cleaner); err != nil {
		log.Error(err, "unable to fetch CronExecutionCleaner")
		return ctrl.Result{}, client.IgnoreNotFound(err)
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

	err := r.List(
		ctx,
		&jobList, client.InNamespace(cleaner.Spec.Namespace),
	)
	if err != nil {
		log.Error(err, "unable to list Jobs for CronExecutionCleaner")
		return ctrl.Result{}, err
	}

	ownedJobs := []batchv1.Job{}

	for _, job := range jobList.Items {
		for _, owner := range job.OwnerReferences {
			if owner.Kind == "CronJob" && owner.Name == cleaner.Spec.CronJobName {
				ownedJobs = append(ownedJobs, job)
				break
			}
		}
	}
	log.Info(
		"Found Jobs owned by CronJob",
		"cronJob", cleaner.Spec.CronJobName,
		"count", len(ownedJobs),
	)

	activeJobs := []batchv1.Job{}
	succeededJobs := []batchv1.Job{}
	failedJobs := []batchv1.Job{}

	for _, job := range ownedJobs {
		switch {
		case job.Status.Active > 0:
			activeJobs = append(activeJobs, job)

		case job.Status.Succeeded > 0:
			succeededJobs = append(succeededJobs, job)

		case job.Status.Failed > 0:
			failedJobs = append(failedJobs, job)
		}
	}
	log.Info(
		"Job classification",
		"active", len(activeJobs),
		"succeeded", len(succeededJobs),
		"failed", len(failedJobs),
	)
	stuckJobs := []batchv1.Job{}
	if cleaner.Spec.CleanupStuck.Enabled {
		now := time.Now()
		stuckAfter := cleaner.Spec.CleanupStuck.StuckAfter.Duration

		for _, job := range activeJobs {
			if job.Status.StartTime == nil {
				continue
			}

			if now.Sub(job.Status.StartTime.Time) > stuckAfter {
				stuckJobs = append(stuckJobs, job)
			}
		}

		log.Info(
			"Stuck job detection",
			"enabled", true,
			"stuckAfter", stuckAfter.String(),
			"count", len(stuckJobs),
		)

		// Retention logic for succeeded jobs
		sort.Slice(succeededJobs, func(i, j int) bool {
			if succeededJobs[i].Status.StartTime == nil {
				return false
			}
			if succeededJobs[j].Status.StartTime == nil {
				return true
			}
			return succeededJobs[i].Status.StartTime.After(
				succeededJobs[j].Status.StartTime.Time,
			)
		})

		retainSucceeded := cleaner.Spec.Retain.SuccessfulJobs
		excessSucceeded := []batchv1.Job{}

		if len(succeededJobs) > retainSucceeded {
			excessSucceeded = succeededJobs[retainSucceeded:]
		}

		log.Info(
			"Succeeded job retention evaluation",
			"retain", retainSucceeded,
			"total", len(succeededJobs),
			"excess", len(excessSucceeded),
		)

		// Retention logic for failed jobs
		sort.Slice(failedJobs, func(i, j int) bool {
			if failedJobs[i].Status.StartTime == nil {
				return false
			}
			if failedJobs[j].Status.StartTime == nil {
				return true
			}
			return failedJobs[i].Status.StartTime.After(
				failedJobs[j].Status.StartTime.Time,
			)
		})

		retainFailed := cleaner.Spec.Retain.FailedJobs
		excessFailed := []batchv1.Job{}

		if len(failedJobs) > retainFailed {
			excessFailed = failedJobs[retainFailed:]
		}

		log.Info(
			"Failed job retention evaluation",
			"retain", retainFailed,
			"total", len(failedJobs),
			"excess", len(excessFailed),
		)

		if len(stuckJobs) > 0 {
			deletedJobs := 0

			for _, job := range stuckJobs {
				log.Info(
					"Deleting stuck Job",
					"job", job.Name,
				)

				policy := metav1.DeletePropagationBackground

				if err := r.Delete(
					ctx,
					&job,
					&client.DeleteOptions{
						PropagationPolicy: &policy,
					},
				); err != nil {
					log.Error(err, "Failed to delete stuck Job", "job", job.Name)
					return ctrl.Result{}, err
				}

				deletedJobs++
			}
			if deletedJobs > 0 {
				now := metav1.Now()

				cleaner.Status.LastRunTime = &now
				cleaner.Status.JobsDeleted += deletedJobs
				cleaner.Status.PodsDeleted += deletedJobs // 1 pod per job in our setup

				if err := r.Status().Update(ctx, &cleaner); err != nil {
					log.Error(err, "Failed to update CronExecutionCleaner status")
					return ctrl.Result{}, err
				}
			}
		}
		deletedSucceeded := 0

		for _, job := range excessSucceeded {
			log.Info(
				"Deleting excess succeeded Job",
				"job", job.Name,
			)

			policy := metav1.DeletePropagationBackground

			if err := r.Delete(
				ctx,
				&job,
				&client.DeleteOptions{
					PropagationPolicy: &policy,
				},
			); err != nil {
				log.Error(err, "Failed to delete succeeded Job", "job", job.Name)
				return ctrl.Result{}, err
			}

			deletedSucceeded++
		}

		deletedFailed := 0

		for _, job := range excessFailed {
			log.Info(
				"Deleting excess failed Job",
				"job", job.Name,
			)

			policy := metav1.DeletePropagationBackground

			if err := r.Delete(
				ctx,
				&job,
				&client.DeleteOptions{
					PropagationPolicy: &policy,
				},
			); err != nil {
				log.Error(err, "Failed to delete failed Job", "job", job.Name)
				return ctrl.Result{}, err
			}

			deletedFailed++
		}

		totalDeleted := deletedSucceeded + deletedFailed

		if totalDeleted > 0 {
			now := metav1.Now()

			cleaner.Status.LastRunTime = &now
			cleaner.Status.JobsDeleted += totalDeleted
			cleaner.Status.PodsDeleted += totalDeleted

			if err := r.Status().Update(ctx, &cleaner); err != nil {
				log.Error(err, "Failed to update CronExecutionCleaner status")
				return ctrl.Result{}, err
			}
		}
	}
	return ctrl.Result{
		RequeueAfter: cleaner.Spec.RunInterval.Duration,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CronExecutionCleanerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lifecyclev1alpha1.CronExecutionCleaner{}).
		Complete(r)
}
