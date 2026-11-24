// Package admission implements a minimal ValidatingAdmissionWebhook for
// McpSource/McpConsumer objects. It has no Kubernetes client dependency and is
// unit-testable with AdmissionReview JSON.
package admission

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"dual-era-mcp-gateway/internal/policy"
	"dual-era-mcp-gateway/internal/source"
)

// Review is admission.k8s.io/v1 AdmissionReview (request side).
type Review struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Request    *Request `json:"request,omitempty"`
}

// Request is the subset of AdmissionRequest used by this webhook.
type Request struct {
	UID    string          `json:"uid"`
	Kind   Kind            `json:"kind"`
	Object json.RawMessage `json:"object,omitempty"`
}

// Kind is GroupVersionKind.
type Kind struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// Response is admission.k8s.io/v1 AdmissionResponse.
type Response struct {
	UID     string  `json:"uid"`
	Allowed bool    `json:"allowed"`
	Status  *Status `json:"status,omitempty"`
}

// Status is metav1.Status (subset).
type Status struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ReviewResponse is the webhook response.
type ReviewResponse struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Response   Response `json:"response"`
}

var tenantRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// Handler validates McpSource/McpConsumer specs.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var review Review
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := dec.Decode(&review); err != nil || review.Request == nil {
			http.Error(w, "bad AdmissionReview", http.StatusBadRequest)
			return
		}
		resp := ReviewResponse{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
			Response:   Response{UID: review.Request.UID, Allowed: true},
		}
		if msg := validate(review.Request); msg != "" {
			resp.Response.Allowed = false
			resp.Response.Status = &Status{Code: http.StatusUnprocessableEntity, Message: msg}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func validate(req *Request) string {
	kind := req.Kind.Kind
	if kind == "" {
		kind = req.Kind.Group
	}
	switch kind {
	case "McpSource", "mcpsource":
		var obj struct {
			Spec source.SourceSpec `json:"spec"`
		}
		if err := json.Unmarshal(req.Object, &obj); err != nil {
			return fmt.Sprintf("decode McpSource: %v", err)
		}
		if err := source.Validate([]source.SourceSpec{obj.Spec}); err != nil {
			return err.Error()
		}
	case "McpConsumer", "mcpconsumer":
		var obj struct {
			Spec policy.Consumer `json:"spec"`
		}
		if err := json.Unmarshal(req.Object, &obj); err != nil {
			return fmt.Sprintf("decode McpConsumer: %v", err)
		}
		if !tenantRE.MatchString(obj.Spec.Tenant) {
			return "tenant must match ^[a-z0-9-]{1,32}$"
		}
		for _, k := range obj.Spec.APIKeys {
			if len(k.SHA256) != 64 {
				return fmt.Sprintf("api key %q must be a 64-char sha256", k.Name)
			}
		}
	default:
		return fmt.Sprintf("unsupported kind %q", kind)
	}
	return ""
}
