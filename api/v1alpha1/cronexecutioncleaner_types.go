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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CronExecutionCleanerSpec defines the desired state of CronExecutionCleaner
type CronExecutionCleanerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Namespace in which the target the CronJob exists
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Name of the CronJob whose executions should be cleaned
	// +kubebuilder:validation:MinLength=1
	CronJobName string `json:"cronJobName"`

	// Retention policy for completed Jobs
	Retain RetentionPolicy `json:"retain"`

	// Configuration for cleaning stuck Jobs
	CleanupStuck CleanupStuckPolicy `json:"cleanupStuck"`

	// Interval at which cleanup logic runs
	RunInterval metav1.Duration `json:"runInterval"`
}

// CronExecutionCleanerStatus defines the observed state of CronExecutionCleaner
type CronExecutionCleanerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Last time the cleanup ran
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// Total number of Jobs deleted
	JobsDeleted int `json:"jobsDeleted,omitempty"`

	// Total number of Pods deleted
	PodsDeleted int `json:"podsDeleted,omitempty"`

	// Current state of the cleaner
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CronExecutionCleaner is the Schema for the cronexecutioncleaners API
type CronExecutionCleaner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CronExecutionCleanerSpec   `json:"spec,omitempty"`
	Status CronExecutionCleanerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CronExecutionCleanerList contains a list of CronExecutionCleaner
type CronExecutionCleanerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronExecutionCleaner `json:"items"`
}

type RetentionPolicy struct {
	// Number of successful Jobs to retain
	// +kubebuilder:validation:Minimum=0
	SuccessfulJobs int `json:"successfulJobs"`

	// Number of failed Jobs to retain
	// +kubebuilder:validation:Minimum=0
	FailedJobs int `json:"failedJobs"`
}

type CleanupStuckPolicy struct {
	// Whether stuck job cleanup is enabled
	Enabled bool `json:"enabled"`

	// Duration after which a running Job is considered stuck
	StuckAfter metav1.Duration `json:"stuckAfter"`
}

func init() {
	SchemeBuilder.Register(&CronExecutionCleaner{}, &CronExecutionCleanerList{})
}
