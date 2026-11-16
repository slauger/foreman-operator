package controller

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

const componentCron = "cron"

// recurringTasks are the rake tasks and schedules of /etc/cron.d/foreman in the Foreman RPM.
var recurringTasks = []struct {
	period, schedule string
}{
	{"hourly", "30 * * * *"},
	{"daily", "0 2 * * *"},
	{"weekly", "0 5 * * 0"},
	{"monthly", "0 3 1 * *"},
}

func cronJobName(f *foremanv1alpha1.Foreman, period string) string {
	return f.Name + "-cron-" + period
}

// cronJob runs one period of the Foreman recurring tasks, such as report expiry and session
// cleanup. Runs never overlap, and a missed run is skipped after ten minutes.
func cronJob(f *foremanv1alpha1.Foreman, images Images, caps Capabilities, period, schedule string, suspend bool) *batchv1.CronJob {
	ctr := corev1.Container{
		Name:      componentCron,
		Command:   []string{"bin/rails", "cron:" + period},
		Env:       foremanEnv(f, 5),
		Resources: defaultResources(corev1.ResourceRequirements{}, "100m", "512Mi", "1536Mi"),
	}
	podSpec := foremanPodSpec(f, images, caps, ctr)
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	labels := commonLabels(foremanAppName, f.Name, componentCron)
	cj := &batchv1.CronJob{}
	cj.Name = cronJobName(f, period)
	cj.Namespace = f.Namespace
	cj.Labels = labels
	cj.Spec = batchv1.CronJobSpec{
		Schedule:                   schedule,
		Suspend:                    new(suspend),
		ConcurrencyPolicy:          batchv1.ForbidConcurrent,
		StartingDeadlineSeconds:    new(int64(600)),
		SuccessfulJobsHistoryLimit: new(int32(1)),
		FailedJobsHistoryLimit:     new(int32(3)),
		JobTemplate: batchv1.JobTemplateSpec{
			Spec: batchv1.JobSpec{
				BackoffLimit: new(int32(1)),
				Template:     corev1.PodTemplateSpec{Spec: podSpec},
			},
		},
	}
	cj.Spec.JobTemplate.Labels = labels
	cj.Spec.JobTemplate.Spec.Template.Labels = labels
	return cj
}

// ensureCronJobs applies the CronJobs of all periods. They are suspended while a migration
// stops the writers, since a running rake task writes to the database.
func (r *ForemanReconciler) ensureCronJobs(ctx context.Context, f *foremanv1alpha1.Foreman, suspend bool) error {
	for _, task := range recurringTasks {
		desired := cronJob(f, r.Images, r.Capabilities, task.period, task.schedule, suspend)
		cj := &batchv1.CronJob{}
		cj.Name = desired.Name
		cj.Namespace = desired.Namespace
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cj, func() error {
			cj.Labels = desired.Labels
			cj.Spec = desired.Spec
			return controllerutil.SetControllerReference(f, cj, r.Scheme)
		}); err != nil {
			return err
		}
	}
	return nil
}
