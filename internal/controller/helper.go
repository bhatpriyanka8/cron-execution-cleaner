package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	lifecyclev1alpha1 "github.com/bhatpriyanka8/cron-execution-cleaner/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func validateSpec(ctx context.Context, cleaner *lifecyclev1alpha1.CronExecutionCleaner) error {

	// Validate Run Interval is at least 1 second or more
	if cleaner.Spec.RunInterval.Duration < time.Second {
		return fmt.Errorf("spec.runInterval must be at least 1s")
	}

	// Validate Retention Policy is non-negative
	if cleaner.Spec.Retain.SuccessfulJobs < 0 {
		return fmt.Errorf("spec.retain.successfulJobs cannot be negative")
	}
	if cleaner.Spec.Retain.FailedJobs < 0 {
		return fmt.Errorf("spec.retain.failedJobs cannot be negative")
	}
	// Validate Cleanup Stuck Policy if enabled, is at least 1 second or more
	if cleaner.Spec.CleanupStuck.Enabled &&
		cleaner.Spec.CleanupStuck.StuckAfter.Duration < time.Second {
		return fmt.Errorf("spec.cleanupStuck.stuckAfter must be at least 1s when enabled")

	}
	return nil
}

func setCondition(
	cleaner *lifecyclev1alpha1.CronExecutionCleaner,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&cleaner.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

func detectStuckJobs(
	jobs []batchv1.Job,
	stuckAfter time.Duration,
	now time.Time,
) []batchv1.Job {
	var stuckJobs []batchv1.Job

	for _, job := range jobs {
		if job.Status.StartTime == nil {
			continue
		}

		if now.Sub(job.Status.StartTime.Time) > stuckAfter {
			stuckJobs = append(stuckJobs, job)
		}
	}
	return stuckJobs
}

func excessJobs(
	jobs []batchv1.Job,
	retainCount int,
) []batchv1.Job {
	// Sort jobs by start time in descending order (newest first)
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Status.StartTime == nil {
			return false
		}
		if jobs[j].Status.StartTime == nil {
			return true
		}
		return jobs[i].Status.StartTime.After(
			jobs[j].Status.StartTime.Time,
		)
	})

	// Return excess jobs (those beyond the retain count)
	if len(jobs) > retainCount {
		return jobs[retainCount:]
	}
	return []batchv1.Job{}
}

func filterJobsByOwner(jobs []batchv1.Job, cronJobName string) []batchv1.Job {
	var ownedJobs []batchv1.Job

	for _, job := range jobs {
		for _, owner := range job.OwnerReferences {
			if owner.Kind == "CronJob" && owner.Name == cronJobName {
				ownedJobs = append(ownedJobs, job)
				break
			}
		}
	}
	return ownedJobs
}
func classifyJobs(jobs []batchv1.Job) (active, succeeded, failed []batchv1.Job) {
	for _, job := range jobs {
		switch {
		case job.Status.Active > 0:
			active = append(active, job)

		case job.Status.Succeeded > 0:
			succeeded = append(succeeded, job)

		case job.Status.Failed > 0:
			failed = append(failed, job)
		}
	}
	return active, succeeded, failed
}

func (r *CronExecutionCleanerReconciler) deleteJobs(
	ctx context.Context,
	jobs []batchv1.Job,
	jobType string,
) int {
	logger := ctrl.LoggerFrom(ctx)
	deletedCount := 0

	policy := metav1.DeletePropagationBackground
	for _, job := range jobs {
		logger.Info("Deleting job", "type", jobType, "job", job.Name)
		if err := r.Delete(ctx, &job, &client.DeleteOptions{PropagationPolicy: &policy}); err != nil {
			logger.Error(err, "Failed to delete job", "type", jobType, "job", job.Name)
			continue
		}
		deletedCount++
	}
	return deletedCount
}
