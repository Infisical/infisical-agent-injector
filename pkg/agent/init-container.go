package agent

import (
	"fmt"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
)

func (a *Agent) ContainerInitSidecar() (corev1.Container, error) {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      util.ContainerWorkDirVolumeName,
			MountPath: util.ContainerWorkDirVolumeMountPath,
			ReadOnly:  false,
		},
		{
			Name:      a.serviceAccountTokenVolume.Name,
			MountPath: a.serviceAccountTokenVolume.MountPath,
			ReadOnly:  true,
		},
	}
	volumeMounts = append(volumeMounts, a.ContainerVolumeMounts()...)

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      util.ContainerAgentConfigVolumeName,
		MountPath: util.ContainerAgentConfigVolumeMountPath,
		ReadOnly:  false, // Changed to false to allow writing
	})

	parsedAgentConfig, err := util.BuildAgentConfigFromConfigMap(a.configMap, true)
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

	script := []string{
		"#!/bin/sh",
		"set -ex", // -x to trace command execution
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
		fmt.Sprintf("timeout 180s infisical agent --config %s/agent-config.yaml || { echo \"Agent failed with exit code $?\"; exit 1; }", util.ContainerAgentConfigVolumeMountPath),
	}...)

	resources, err := util.CreateDefaultResources()
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to create resources: %w", err)
	}

	newContainer := corev1.Container{
		Name:      util.InitContainerName,
		Image:     util.ContainerImage,
		Resources: resources,

		VolumeMounts: volumeMounts,
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{strings.Join(script, "\n")},
	}

	return newContainer, nil

}
