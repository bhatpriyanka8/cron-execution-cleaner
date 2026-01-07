package controller

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClassifyJobs(t *testing.T) {
	jobs := []batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "active-job"},
			Status:     batchv1.JobStatus{Active: 1},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "succeeded-job"},
			Status:     batchv1.JobStatus{Succeeded: 1},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "failed-job"},
			Status:     batchv1.JobStatus{Failed: 1},
		},
	}

	active, succeeded, failed := classifyJobs(jobs)

	if len(active) != 1 || active[0].Name != "active-job" {
		t.Fatalf("expected 1 active job, got %d", len(active))
	}
	if len(succeeded) != 1 || succeeded[0].Name != "succeeded-job" {
		t.Fatalf("expected 1 succeeded job, got %d", len(succeeded))
	}
	if len(failed) != 1 || failed[0].Name != "failed-job" {
		t.Fatalf("expected 1 failed job, got %d", len(failed))
	}
}

func TestFilterJobsByOwner(t *testing.T) {
	jobs := []batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job-1",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "CronJob", Name: "my-cronjob"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job-2",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "CronJob", Name: "other-cronjob"},
				},
			},
		},
	}

	filtered := filterJobsByOwner(jobs, "my-cronjob")

	if len(filtered) != 1 || filtered[0].Name != "job-1" {
		t.Fatalf("expected 1 filtered job, got %d", len(filtered))
	}
}
func TestDetectStuckJobs(t *testing.T) {
	now := time.Now()

	job := batchv1.Job{
		Status: batchv1.JobStatus{
			Active: 1,
			StartTime: &metav1.Time{
				Time: now.Add(-2 * time.Hour),
			},
		},
	}

	jobs := []batchv1.Job{job}

	stuck := detectStuckJobs(jobs, time.Hour, now)

	if len(stuck) != 1 {
		t.Fatalf("expected 1 stuck job, got %d", len(stuck))
	}
}

func TestExcessJobs(t *testing.T) {
	jobs := []batchv1.Job{
		{ObjectMeta: metav1.ObjectMeta{Name: "job-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "job-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "job-3"}},
	}

	excess := excessJobs(jobs, 2)

	if len(excess) != 1 {
		t.Fatalf("expected 1 excess job, got %d", len(excess))
	}

	if excess[0].Name != "job-3" {
		t.Fatalf("unexpected job selected for deletion")
	}
}

func TestExcessJobsWithStartTime(t *testing.T) {
	now := time.Now()
	jobs := []batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job-1"},
			Status: batchv1.JobStatus{
				StartTime: &metav1.Time{Time: now.Add(-3 * time.Hour)},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job-2"},
			Status: batchv1.JobStatus{
				StartTime: &metav1.Time{Time: now.Add(-2 * time.Hour)},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "job-3"},
			Status: batchv1.JobStatus{
				StartTime: &metav1.Time{Time: now},
			},
		},
	}

	excess := excessJobs(jobs, 1)

	if len(excess) != 2 {
		t.Fatalf("expected 2 excess jobs, got %d", len(excess))
	}
	// After sorting by newest first, excess jobs should be the older ones
	// Newest job (job-3) is retained, so job-2 and job-1 are excess
	if excess[0].Name != "job-2" {
		t.Fatalf("expected job-2 first, got %s", excess[0].Name)
	}
	if excess[1].Name != "job-1" {
		t.Fatalf("expected job-1 second, got %s", excess[1].Name)
	}
}

func TestExcessJobsNone(t *testing.T) {
	jobs := []batchv1.Job{
		{ObjectMeta: metav1.ObjectMeta{Name: "job-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "job-2"}},
	}

	excess := excessJobs(jobs, 5)

	if len(excess) != 0 {
		t.Fatalf("expected no excess jobs, got %d", len(excess))
	}
}
