package controller

import (
	"context"
	"fmt"
	"time"

	lifecyclev1alpha1 "github.com/bhatpriyanka8/cron-execution-cleaner/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *CronExecutionCleanerReconciler) validateSpec(ctx context.Context, cleaner *lifecyclev1alpha1.CronExecutionCleaner) error {

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
