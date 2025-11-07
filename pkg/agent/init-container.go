package agent

import (
	"fmt"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

func (a *Agent) ContainerInitSidecar() (corev1.Container, error) {

	agentConfigVolumeMountPath := util.LinuxContainerWorkDirVolumeMountPath
	if a.isWindows {
		agentConfigVolumeMountPath = util.WindowsContainerWorkDirVolumeMountPath
	}

	volumeMounts := []corev1.VolumeMount{}

	if !a.isWindows {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      a.serviceAccountTokenVolume.Name,
			MountPath: a.serviceAccountTokenVolume.MountPath,
			ReadOnly:  true,
		})
	}
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      util.ContainerWorkDirVolumeName,
		MountPath: agentConfigVolumeMountPath,
		ReadOnly:  false,
	})

	volumeMounts = append(volumeMounts, a.ContainerVolumeMounts(volumeMounts)...)

	script, err := util.BuildAgentScript(*a.configMap, true, a.isWindows)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to build agent script: %w", err)
	}

	resources, err := util.CreateDefaultResources(a.isWindows)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to create resources: %w", err)
	}

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
		Args:         []string{script},
	}

	return newContainer, nil

}
