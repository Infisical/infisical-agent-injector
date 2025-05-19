package agent

import (
	"fmt"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func (a *Agent) ContainerInitSidecar() (corev1.Container, error) {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      util.InitContainerVolumeMountName,
			MountPath: util.InitContainerVolumeMountPath,
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
		Name:      util.InitContainerAgentConfigVolumeName,
		MountPath: util.InitContainerAgentConfigVolumeMountPath,
		ReadOnly:  false, // Changed to false to allow writing
	})

	parsedAgentConfig := util.BuildAgentConfigFromConfigMap(a.configMap, a.injectMode)

	agentConfigYaml, err := yaml.Marshal(parsedAgentConfig)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to marshal yaml agent config: %w", err)
	}

	script := []string{
		"#!/bin/sh",
		"set -ex", // -x to trace command execution
		"",
		"# Write config file to volume",
		fmt.Sprintf("cat > %s/agent-config.yaml << 'EOF'", util.InitContainerAgentConfigVolumeMountPath),
		string(agentConfigYaml),
		"EOF",
		"",
		"# Write identity id to volume",
		fmt.Sprintf("mkdir -p %s", util.InitContainerAgentConfigVolumeMountPath),
		fmt.Sprintf("echo \"%s\" > %s/identity-id", a.configMap.Infisical.Auth.Config.IdentityID, util.InitContainerAgentConfigVolumeMountPath),
		"",
		"# Run the agent",
		"echo \"Starting infisical agent...\"",
		fmt.Sprintf("timeout 180s infisical agent --config %s/agent-config.yaml || { echo \"Agent failed with exit code $?\"; exit 1; }", util.InitContainerAgentConfigVolumeMountPath),
	}

	resources, err := createResources()
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to create resources: %w", err)
	}

	newContainer := corev1.Container{
		Name:      util.InitContainerName,
		Image:     util.InitContainerImage,
		Resources: resources,

		VolumeMounts: volumeMounts,
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{strings.Join(script, "\n")},
	}

	return newContainer, nil

}

func createResources() (corev1.ResourceRequirements, error) {
	// currently has the same resources and limits as the infisical gateway
	// we might want to make this configurable in the future
	resources := corev1.ResourceRequirements{}

	// create the limits
	limits := corev1.ResourceList{}
	cpuLimit, err := resource.ParseQuantity("500m")
	if err != nil {
		return resources, fmt.Errorf("failed to parse CPU limit: %w", err)
	}
	memoryLimit, err := resource.ParseQuantity("128Mi")
	if err != nil {
		return resources, fmt.Errorf("failed to parse memory limit: %w", err)
	}
	limits[corev1.ResourceCPU] = cpuLimit
	limits[corev1.ResourceMemory] = memoryLimit

	// create the requests
	requests := corev1.ResourceList{}
	cpuRequest, err := resource.ParseQuantity("100m")
	if err != nil {
		return resources, fmt.Errorf("failed to parse CPU request: %w", err)
	}
	memoryRequest, err := resource.ParseQuantity("128Mi")
	if err != nil {
		return resources, fmt.Errorf("failed to parse memory request: %w", err)
	}
	requests[corev1.ResourceCPU] = cpuRequest
	requests[corev1.ResourceMemory] = memoryRequest

	// set the limits and requests on the resource requirements
	resources.Limits = limits
	resources.Requests = requests

	return resources, nil
}
