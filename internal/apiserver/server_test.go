package apiserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	v1alpha1 "github.com/dcm-project/3-tier-demo-service-provider/api/v1alpha1"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
)

const bareSpec = `{"database":{"engine":"postgres","version":"16"},"app":{"image":"app:v1"},"web":{"image":"nginx:alpine"}}`

func TestSpecWrapMiddleware(t *testing.T) {
	var receivedBody []byte
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	handler := specWrapMiddleware(inner)

	t.Run("does not double-wrap", func(t *testing.T) {
		wrapped := `{"spec":{"database":{"engine":"postgres","version":"16"},"app":{"image":"app:v1"},"web":{"image":"nginx:alpine"}}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/three-tier-apps", bytes.NewBufferString(wrapped))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		var got map[string]json.RawMessage
		if err := json.Unmarshal(receivedBody, &got); err != nil {
			t.Fatalf("unmarshal received body: %v", err)
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(got["spec"], &inner); err != nil {
			t.Fatalf("unmarshal inner spec: %v", err)
		}
		if _, ok := inner["spec"]; ok {
			t.Fatal("body was double-wrapped")
		}
	})

	t.Run("ignores GET requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/three-tier-apps", bytes.NewBufferString(bareSpec))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		var got map[string]json.RawMessage
		if err := json.Unmarshal(receivedBody, &got); err != nil {
			t.Fatalf("unmarshal received body: %v", err)
		}
		if _, ok := got["spec"]; ok {
			t.Fatal("GET body should not be wrapped")
		}
	})

	t.Run("ignores other paths", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/other", bytes.NewBufferString(bareSpec))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		var got map[string]json.RawMessage
		if err := json.Unmarshal(receivedBody, &got); err != nil {
			t.Fatalf("unmarshal received body: %v", err)
		}
		if _, ok := got["spec"]; ok {
			t.Fatal("body on other path should not be wrapped")
		}
	})
}

func TestBareSpecOpenAPIValidation(t *testing.T) {
	swagger, err := v1alpha1.GetSwagger()
	if err != nil {
		t.Fatalf("load swagger: %v", err)
	}
	validate := oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapimiddleware.Options{
		SilenceServersWarning: true,
	})
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	t.Run("bare spec rejected without middleware", func(t *testing.T) {
		handler := validate(ok)
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/three-tier-apps", bytes.NewBufferString(bareSpec))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 without middleware, got %d", w.Code)
		}
	})

	t.Run("bare spec passes with middleware", func(t *testing.T) {
		handler := specWrapMiddleware(validate(ok))
		req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/three-tier-apps", bytes.NewBufferString(bareSpec))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 with middleware, got %d: %s", w.Code, w.Body.String())
		}
	})
}
