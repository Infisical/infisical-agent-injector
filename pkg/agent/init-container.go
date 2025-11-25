package agent

import (
	"fmt"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

func (a *Agent) ContainerInitSidecar() (corev1.Container, error) {
	volumeMounts := []corev1.VolumeMount{}

	if !a.isWindows {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      a.serviceAccountTokenVolume.Name,
			MountPath: a.serviceAccountTokenVolume.MountPath,
			ReadOnly:  true,
		})
	}

	volumeMounts = append(volumeMounts, a.ContainerVolumeMounts(volumeMounts)...)

	script, envVars, err := util.BuildAgentScript(*a.configMap, true, a.isWindows, a.injectMode, a.cachingEnabled, a.pod.Annotations)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to build agent script: %w", err)
	}

	resources := a.ResourceRequirements()

	command := []string{"/bin/sh", "-c"}
	if a.isWindows {
		command = []string{"pwsh.exe", "-Command"}
	}

	newContainer := corev1.Container{
		Name:         util.InitContainerName,
		Image:        a.agentImage,
		Resources:    resources,
		VolumeMounts: volumeMounts,
		Command:      command,
		Env:          envVars,
		Args:         []string{script},
	}

	return newContainer, nil

}
