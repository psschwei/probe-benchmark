package main

import (
	"github.com/psschwei/podspeed/pkg/podspeed"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "standard-pod",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "standard-pod",
					Image: "k8s.gcr.io/busybox",
					Args:  []string{"/bin/sh", "-c", "sleep 1; touch /tmp/healthy; sleep 60; rm -rf /tmp/healthy"},
					StartupProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"cat", "/tmp/healthy"},
							},
						},
						InitialDelaySeconds: 0,
						PeriodSeconds:       1,
						FailureThreshold:    30,
					},
				},
			},
		},
	}

	podspeed.Run("default", pod, "basic", "", 10, false, true, false, false)
}
