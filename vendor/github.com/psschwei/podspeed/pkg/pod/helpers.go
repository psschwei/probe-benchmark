package pod

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

func IsConditionTrue(p *corev1.Pod, condType corev1.PodConditionType) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == condType && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func LastContainerStartedTime(p *corev1.Pod) time.Time {
	var last time.Time
	for _, cond := range p.Status.ContainerStatuses {
		if last.Before(cond.State.Running.StartedAt.Time) {
			last = cond.State.Running.StartedAt.Time
		}
	}
	return last
}
