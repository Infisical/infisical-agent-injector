package agent

import (
	"fmt"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
)

func (a *Agent) ContainerSidecar() (corev1.Container, error) {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      a.serviceAccountTokenVolume.Name,
			MountPath: a.serviceAccountTokenVolume.MountPath,
			ReadOnly:  true,
		},
		{
			Name:      util.ContainerWorkDirVolumeName,
			MountPath: util.ContainerWorkDirVolumeMountPath,
			ReadOnly:  false,
		},
	}
	volumeMounts = append(volumeMounts, a.ContainerVolumeMounts()...)

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      util.ContainerAgentConfigVolumeName,
		MountPath: util.ContainerAgentConfigVolumeMountPath,
		ReadOnly:  false,
	})

	parsedAgentConfig, err := util.BuildAgentConfigFromConfigMap(a.configMap, false)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to build agent config: %w", err)
	}

	agentConfigYaml, err := yaml.Marshal(parsedAgentConfig)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to marshal yaml agent config: %w", err)
	}

	filePathCreationScript, err := util.GetFilePathCreationScript(*a.configMap)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to get file path creation script: %w", err)
	}

	resources, err := util.CreateDefaultResources()
	if err != nil {
		return corev1.Container{}, err
	}

	script := []string{
		"#!/bin/sh",
		"set -ex",
		"",
		"# Write config file to volume",
		fmt.Sprintf("cat > %s/agent-config.yaml << 'EOF'", util.ContainerAgentConfigVolumeMountPath),
		string(agentConfigYaml),
		"EOF",
		"",
		"# Write identity id to volume",
		fmt.Sprintf("mkdir -p %s", util.ContainerAgentConfigVolumeMountPath),
	}

	script = append(script, filePathCreationScript...)

	script = append(script, []string{
		"",
		"# Run the agent",
		"echo \"Starting infisical agent...\"",
		fmt.Sprintf("infisical agent --config %s/agent-config.yaml", util.ContainerAgentConfigVolumeMountPath),
	}...)

	lifecycle := a.createLifecycle()

	newContainer := corev1.Container{
		Name:         "infisical-agent",
		Image:        util.ContainerImage,
		Resources:    resources,
		VolumeMounts: volumeMounts,
		Lifecycle:    &lifecycle,
		Command:      []string{"/bin/sh", "-ec"},
		Args:         []string{strings.Join(script, "\n")},
	}

	return newContainer, nil
}
