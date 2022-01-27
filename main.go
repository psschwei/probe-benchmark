package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load kubernetes config.
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), nil).ClientConfig()
	if err != nil {
		log.Fatalln("Failed to load config", err)
	}

	// Create kubernetes client.
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln("Failed to create Kubernetes client", err)
	}

	standardPod := &corev1.Pod{
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

	pod, err := kube.CoreV1().Pods("default").Create(ctx, standardPod, metav1.CreateOptions{})
	if err != nil {
		log.Fatalln("")
	}

	fmt.Printf("Pod %s created successfully", pod.Name)
}
