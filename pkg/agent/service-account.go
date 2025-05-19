package agent

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type ServiceAccountTokenVolume struct {
	Name      string
	MountPath string
	TokenPath string
}

func getServiceAccountTokenVolume(pod *corev1.Pod) (*ServiceAccountTokenVolume, error) {
	for _, container := range pod.Spec.Containers {
		for _, volumes := range container.VolumeMounts {
			if strings.Contains(volumes.MountPath, "serviceaccount") {
				return &ServiceAccountTokenVolume{
					Name:      volumes.Name,
					MountPath: volumes.MountPath,
					TokenPath: "token",
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no service account token volume found")
}
