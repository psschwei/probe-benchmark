package main

import (
	"github.com/psschwei/podspeed/pkg/podspeed"
	corev1 "k8s.io/api/core/v1"
)

func main() {

	pod := &corev1.Pod{}

	podspeed.Run("default", pod, "basic", "", 10, false, true, false, false)
}
