package injector

import (
	"context"
	"fmt"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func IsInjectable(pod corev1.Pod) bool {
	return pod.Annotations[util.InjectAnnotation] == "true"
}

func GetConfigMap(client kubernetes.Interface, pod corev1.Pod) (*util.ConfigMap, error) {
	configMapName := pod.Annotations[util.AnnotationAgentConfigMap]
	if configMapName == "" {
		return nil, fmt.Errorf("no config map found")
	}

	namespace := pod.Namespace
	configMap, err := client.CoreV1().ConfigMaps(namespace).Get(context.TODO(), configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s in namespace %s: %w", configMapName, namespace, err)
	}

	// parse the config map
	var parsedConfigMap util.ConfigMap
	if err := yaml.Unmarshal([]byte(configMap.Data["config.yaml"]), &parsedConfigMap); err != nil {
		return nil, fmt.Errorf("failed to parse ConfigMap data: %w", err)
	}

	return &parsedConfigMap, nil
}
