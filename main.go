package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Infisical/infisical-agent-injector/pkg/injector"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NAMESPACE is set by kubernetes to the namespace where the injector is running
func getNamespace() string {
	namespace := os.Getenv("NAMESPACE")
	if namespace != "" {
		return namespace
	}

	return "default"
}

func handleReady(rw http.ResponseWriter, req *http.Request) {
	// Always ready at this point. The main readiness check is whether
	// there is a TLS certificate. If we reached this point it means we
	// served a TLS certificate.
	rw.WriteHeader(204)
}

func updateWebhookConfig(clientset *kubernetes.Clientset, cert []byte) {
	// Base64 encode the cert
	caBundle := base64.StdEncoding.EncodeToString(cert)

	// Get the webhook configuration
	webhookConfigName := "infisical-agent-injector-cfg"

	// Retry loop for resiliency
	for i := 0; i < 5; i++ {
		log.Printf("Attempting to update webhook config (attempt %d)...", i+1)

		// Create a JSON patch
		type patchValue struct {
			Op    string `json:"op"`
			Path  string `json:"path"`
			Value string `json:"value"`
		}

		// Create a patch for each webhook in the configuration
		// Note: This assumes there's only one webhook entry - adjust if you have more
		patch := []patchValue{
			{
				Op:    "replace",
				Path:  "/webhooks/0/clientConfig/caBundle",
				Value: caBundle,
			},
		}

		patchBytes, err := json.Marshal(patch)
		if err != nil {
			log.Printf("Failed to marshal patch: %v, retrying...", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Apply patch to the webhook configuration
		_, err = clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Patch(
			context.Background(),
			webhookConfigName,
			types.JSONPatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
		if err != nil {
			log.Printf("Failed to patch webhook config: %v, retrying...", err)
			time.Sleep(2 * time.Second)
			continue
		}

		log.Println("Successfully updated webhook configuration with CA bundle")
		return
	}

	log.Println("Warning: Failed to update webhook configuration after multiple attempts")
}

func getKubernetesClient() (*kubernetes.Clientset, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes client config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return clientset, nil
}

func main() {
	log.Println("Starting infisical-agent-injector...")

	kubeClient, err := getKubernetesClient()
	if err != nil {
		log.Fatalf("Failed to get kubernetes client: %v", err)
	}

	// Generate self-signed cert
	log.Printf("Generating self-signed certificate for namespace: %s", getNamespace())
	tlsCert, tlsKey, err := injector.GenerateSelfSignedCert(getNamespace())
	if err != nil {
		log.Fatalf("Failed to generate certificate: %v", err)
	}

	// Setup HTTP handlers
	handler := injector.Handler{
		Client: kubeClient,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", handler.Handle)
	mux.HandleFunc("/health/ready", handleReady)

	// Write certs to temp directory
	certPath := "/tmp/tls"
	log.Printf("Creating directory: %s", certPath)
	err = os.MkdirAll(certPath, 0755)
	if err != nil {
		log.Fatalf("Failed to create directory %s: %v", certPath, err)
	}

	certFile := certPath + "/tls.crt"
	keyFile := certPath + "/tls.key"

	log.Printf("Writing cert to: %s", certFile)
	err = os.WriteFile(certFile, tlsCert, 0644)
	if err != nil {
		log.Fatalf("Failed to write cert file: %v", err)
	}

	log.Printf("Writing key to: %s", keyFile)
	err = os.WriteFile(keyFile, tlsKey, 0644)
	if err != nil {
		log.Fatalf("Failed to write key file: %v", err)
	}

	// Update the webhook configuration with the CA bundle
	go updateWebhookConfig(kubeClient, tlsCert)

	// Start the HTTPS server
	log.Printf("Starting HTTPS server on port 8585...")
	err = http.ListenAndServeTLS(":8585", certFile, keyFile, mux)
	if err != nil {
		log.Fatalf("Failed to start HTTPS server: %v", err)
	}
}
