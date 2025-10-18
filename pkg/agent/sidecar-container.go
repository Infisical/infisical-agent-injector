package agent

import (
	"fmt"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

func (a *Agent) ContainerSidecar() (corev1.Container, error) {
	agentConfigVolumeMountPath := util.LinuxContainerAgentConfigVolumeMountPath
	if a.isWindows {
		agentConfigVolumeMountPath = util.WindowsContainerAgentConfigVolumeMountPath
	}

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

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      util.ContainerAgentConfigVolumeName,
		MountPath: agentConfigVolumeMountPath,
		ReadOnly:  false,
	})

	script, err := util.BuildAgentScript(*a.configMap, a.injectMode, a.isWindows)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to build agent script: %w", err)
	}

	resources, err := util.CreateDefaultResources()
	if err != nil {
		return corev1.Container{}, err
	}
	lifecycle := a.createLifecycle()

	command := []string{"/bin/sh", "-ec"}
	if a.isWindows {
		command = []string{"pwsh.exe", "-Command"}
	}

	newContainer := corev1.Container{
		Name:         "infisical-agent",
		Image:        a.agentImage,
		Resources:    resources,
		VolumeMounts: volumeMounts,
		Lifecycle:    &lifecycle,
		Command:      command,
		Args:         []string{script},
	}

	return newContainer, nil
}
