package injector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"

	"github.com/Infisical/infisical-agent-injector/pkg/agent"
	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"github.com/google/uuid"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
)

var (
	admissionScheme = runtime.NewScheme()
	deserializer    = func() runtime.Decoder {
		codecs := serializer.NewCodecFactory(admissionScheme)
		return codecs.UniversalDeserializer()
	}
)

func init() {
	utilruntime.Must(admissionv1.AddToScheme(admissionScheme))
	utilruntime.Must(v1beta1.AddToScheme(admissionScheme))
}

type Handler struct {
	Client *kubernetes.Clientset
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		msg := fmt.Sprintf("Only application/json is supported, got: %q", contentType)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	var body []byte
	if r.Body != nil {
		var err error
		if body, err = io.ReadAll(r.Body); err != nil {
			msg := fmt.Sprintf("error reading request body: %s", err)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
	}
	if len(body) == 0 {
		msg := "No request body was sent in the request"
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	var (
		mutateResp MutateResponse
		admResp    admissionv1.AdmissionReview
	)

	admReq := unversionedAdmissionReview{}
	admReq.SetGroupVersionKind(admissionv1.SchemeGroupVersion.WithKind("AdmissionReview"))
	_, actualAdmRevGVK, err := deserializer().Decode(body, nil, &admReq)
	if err != nil {
		msg := fmt.Sprintf("error decoding admission request: %s", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	} else {
		mutateResp = h.Mutate(admReq.Request)
		admResp.Response = mutateResp.Resp
	}

	if actualAdmRevGVK == nil || *actualAdmRevGVK == (schema.GroupVersionKind{}) {
		admResp.SetGroupVersionKind(admissionv1.SchemeGroupVersion.WithKind("AdmissionReview"))
	} else {
		admResp.SetGroupVersionKind(*actualAdmRevGVK)
	}

	resp, err := json.Marshal(&admResp)
	if err != nil {
		errorMessage := fmt.Sprintf("error marshalling admission response: %s", err)
		http.Error(w, errorMessage, http.StatusInternalServerError)
		log.Printf("error on request: %s", errorMessage)
		return
	}

	if _, err := w.Write(resp); err != nil {
		errorMessage := fmt.Sprintf("error writing response: %s", err)
		log.Printf("error on request: %s", errorMessage)
		http.Error(w, errorMessage, http.StatusInternalServerError)
		return
	}
}

type MutateResponse struct {
	Resp            *admissionv1.AdmissionResponse
	InjectedInit    bool
	InjectedSidecar bool
}

func randomRequestId() string {
	hash := sha256.New()
	hash.Write([]byte(uuid.New().String()))
	return hex.EncodeToString(hash.Sum(nil))[:10]
}

func (h *Handler) Mutate(req *admissionv1.AdmissionRequest) MutateResponse {
	requestId := randomRequestId()

	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return admissionsApiError(req.UID, err)
	}

	resp := &admissionv1.AdmissionResponse{
		Allowed: true,
		UID:     req.UID,
	}

	log.Printf("[request-id=%s] New create or update mutation request received for pod: %s in namespace: %s. Checking if secrets should be injected..", requestId, pod.Name, pod.Namespace)

	if !IsInjectable(pod) {
		log.Printf("[request-id=%s] Pod %s in namespace %s is not injectable, skipping..", requestId, pod.Name, pod.Namespace)
		return MutateResponse{
			Resp: resp,
		}
	}

	agentConfig, err := GetConfigMap(h.Client, pod)
	if err != nil {
		log.Printf("[request-id=%s] Error getting config map for pod %s in namespace %s: %s", requestId, pod.Name, pod.Namespace, err)
		return admissionsApiError(req.UID, err)
	}

	if slices.Contains(util.KubeSystemNamespaces, pod.Namespace) {
		err := fmt.Errorf("system namespace is not injectable: %s", pod.Namespace)
		log.Printf("[request-id=%s] %s", requestId, err)
		return admissionsApiError(req.UID, err)
	}

	log.Printf("[request-id=%s] Injecting into pod: %s in namespace: %s", requestId, pod.Name, pod.Namespace)

	agent, err := agent.NewAgent(&pod, agentConfig)
	if err != nil {
		log.Printf("[request-id=%s] Error creating agent for pod %s in namespace %s: %s", requestId, pod.Name, pod.Namespace, err)
		return admissionsApiError(req.UID, err)
	}

	err = agent.ValidateConfigMap()
	if err != nil {
		log.Printf("[request-id=%s] Error validating config map for pod %s in namespace %s: %s", requestId, pod.Name, pod.Namespace, err)
		return admissionsApiError(req.UID, err)
	}

	patch, err := agent.PatchPod()
	if err != nil {
		log.Printf("[request-id=%s] Error patching pod %s in namespace %s: %s", requestId, pod.Name, pod.Namespace, err)
		return admissionsApiError(req.UID, err)
	}

	log.Printf("[request-id=%s] Successfully patched pod: %s in namespace: %s", requestId, pod.Name, pod.Namespace)

	resp.Patch = patch
	patchType := admissionv1.PatchTypeJSONPatch
	resp.PatchType = &patchType

	return MutateResponse{
		Resp:            resp,
		InjectedInit:    true,
		InjectedSidecar: false,
	}
}

func admissionsApiError(reqUid types.UID, e error) MutateResponse {
	return MutateResponse{
		Resp: &admissionv1.AdmissionResponse{
			UID: reqUid,
			Result: &metav1.Status{
				Message: e.Error(),
			},
		},
	}
}

type unversionedAdmissionReview struct {
	admissionv1.AdmissionReview
}

var _ runtime.Object = &unversionedAdmissionReview{}
