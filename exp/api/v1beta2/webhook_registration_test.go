/*
Copyright (c) 2026 Oracle and/or its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta2

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/conversion"
)

type registrationTestManager struct {
	ctrl.Manager
	scheme            *runtime.Scheme
	webhookServer     webhook.Server
	converterRegistry conversion.Registry
}

func (m *registrationTestManager) GetConfig() *rest.Config {
	return &rest.Config{}
}

func (m *registrationTestManager) GetScheme() *runtime.Scheme {
	return m.scheme
}

func (m *registrationTestManager) GetWebhookServer() webhook.Server {
	return m.webhookServer
}

func (m *registrationTestManager) GetConverterRegistry() conversion.Registry {
	return m.converterRegistry
}

func TestWebhookRegistrationsServeAdmissionReviews(t *testing.T) {
	tests := []struct {
		name     string
		object   runtime.Object
		resource string
		paths    []string
		setup    func(ctrl.Manager) error
	}{
		{
			name:     "OCIManagedMachinePool",
			object:   &OCIManagedMachinePool{TypeMeta: metav1.TypeMeta{APIVersion: GroupVersion.String(), Kind: "OCIManagedMachinePool"}},
			resource: "ocimanagedmachinepools",
			paths: []string{
				"/mutate-infrastructure-cluster-x-k8s-io-v1beta2-ocimanagedmachinepool",
				"/validate-infrastructure-cluster-x-k8s-io-v1beta2-ocimanagedmachinepool",
			},
			setup: func(mgr ctrl.Manager) error {
				return (&OCIManagedMachinePool{}).SetupWebhookWithManager(mgr)
			},
		},
		{
			name:     "OCIVirtualMachinePool",
			object:   &OCIVirtualMachinePool{TypeMeta: metav1.TypeMeta{APIVersion: GroupVersion.String(), Kind: "OCIVirtualMachinePool"}},
			resource: "ocivirtualmachinepools",
			paths: []string{
				"/mutate-infrastructure-cluster-x-k8s-io-v1beta2-ocivirtualmachinepool",
				"/validate-infrastructure-cluster-x-k8s-io-v1beta2-ocivirtualmachinepool",
			},
			setup: func(mgr ctrl.Manager) error {
				return (&OCIVirtualMachinePool{}).SetupWebhookWithManager(mgr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := AddToScheme(scheme); err != nil {
				t.Fatalf("add API types to scheme: %v", err)
			}

			server := webhook.NewServer(webhook.Options{WebhookMux: http.NewServeMux()})
			mgr := &registrationTestManager{
				scheme:            scheme,
				webhookServer:     server,
				converterRegistry: conversion.NewRegistry(),
			}
			if err := tt.setup(mgr); err != nil {
				t.Fatalf("register production webhook: %v", err)
			}

			for _, path := range tt.paths {
				t.Run(path, func(t *testing.T) {
					serveAdmissionReview(t, server.WebhookMux(), path, tt.resource, tt.object)
				})
			}
		})
	}
}

func serveAdmissionReview(t *testing.T, mux *http.ServeMux, path, resource string, object runtime.Object) {
	t.Helper()

	rawObject, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal admitted object: %v", err)
	}

	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       types.UID("registration-test"),
			Kind:      metav1.GroupVersionKind{Group: GroupVersion.Group, Version: GroupVersion.Version, Kind: object.GetObjectKind().GroupVersionKind().Kind},
			Resource:  metav1.GroupVersionResource{Group: GroupVersion.Group, Version: GroupVersion.Version, Resource: resource},
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: rawObject},
		},
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal admission review: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	// The production registration sets RecoverPanic(false), and ServeMux performs no
	// panic recovery. Any panic in object construction or dispatch fails this test.
	mux.ServeHTTP(responseRecorder, req)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("admission request returned HTTP %d: %s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var response admissionv1.AdmissionReview
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode admission response: %v", err)
	}
	if response.Response == nil {
		t.Fatal("admission response is missing")
	}
	if response.Response.UID != review.Request.UID {
		t.Fatalf("admission response UID = %q, want %q", response.Response.UID, review.Request.UID)
	}
}
