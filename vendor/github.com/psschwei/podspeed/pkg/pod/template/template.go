package template

import (
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func PodConstructorFromYAML(content io.Reader) (func(string, string) *corev1.Pod, error) {
	pod := &corev1.Pod{}

	decoder := yaml.NewYAMLOrJSONDecoder(content, 64)
	if err := decoder.Decode(pod); err != nil {
		return nil, fmt.Errorf("failed to decode YAML content: %w", err)
	}

	return func(ns, name string) *corev1.Pod {
		pod := pod.DeepCopy()

		// Reset metadata
		pod.ObjectMeta = metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		}

		// Reset Status
		pod.Status = corev1.PodStatus{}

		return pod
	}, nil
}

func PodConstructorFromObject(pod *corev1.Pod) (func(string, string) *corev1.Pod, error) {
	return func(ns, name string) *corev1.Pod {
		pod = pod.DeepCopy()

		// Reset metadata
		pod.ObjectMeta = metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		}

		// Reset Status
		pod.Status = corev1.PodStatus{}

		return pod
	}, nil
}
