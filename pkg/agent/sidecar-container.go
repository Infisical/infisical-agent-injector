package agent

import (
	"fmt"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

func (a *Agent) ContainerSidecar() (corev1.Container, error) {
	volumeMounts := []corev1.VolumeMount{}

	if !a.isWindows {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      a.serviceAccountTokenVolume.Name,
			MountPath: a.serviceAccountTokenVolume.MountPath,
			ReadOnly:  true,
		})
	}

	// This will add the secret volume mounts
	volumeMounts = append(volumeMounts, a.ContainerVolumeMounts(volumeMounts)...)

	script, envVars, err := util.BuildAgentScript(*a.configMap, false, a.isWindows, a.injectMode, a.cachingEnabled, a.pod.Annotations)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to build agent script: %w", err)
	}

	resources, err := a.ResourceRequirements()
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to get resource requirements: %w", err)
	}
	lifecycle := a.Lifecycle()

	command := []string{"/bin/sh", "-ec"}
	if a.isWindows {
		command = []string{"pwsh.exe", "-Command"}
	}

	newContainer := corev1.Container{
		Name:         util.SidecarContainerName,
		Image:        a.agentImage,
		Resources:    resources,
		VolumeMounts: volumeMounts,
		Lifecycle:    &lifecycle,
		Command:      command,
		Args:         []string{script},
		Env:          envVars,
	}

	return newContainer, nil
}
