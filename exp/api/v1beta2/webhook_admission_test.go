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
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestCustomWebhooksHandleSerializedAdmissionRequests(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("add v1beta2 types to scheme: %v", err)
	}

	tests := []struct {
		name      string
		kind      string
		object    runtime.Object
		defaulter admission.CustomDefaulter
		validator admission.CustomValidator
	}{
		{
			name:      "OCIManagedMachinePool",
			kind:      "OCIManagedMachinePool",
			object:    &OCIManagedMachinePool{},
			defaulter: &OCIManagedMachinePoolWebhook{},
			validator: &OCIManagedMachinePoolWebhook{},
		},
		{
			name:      "OCIVirtualMachinePool",
			kind:      "OCIVirtualMachinePool",
			object:    &OCIVirtualMachinePool{},
			defaulter: &OCIVirtualMachinePoolWebhook{},
			validator: &OCIVirtualMachinePoolWebhook{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.object.GetObjectKind().SetGroupVersionKind(GroupVersion.WithKind(test.kind))

			t.Run("default", func(t *testing.T) {
				hook := admission.WithCustomDefaulter(scheme, test.object, test.defaulter)
				assertSerializedAdmissionResponse(t, hook, test.object)
			})
			t.Run("validate", func(t *testing.T) {
				hook := admission.WithCustomValidator(scheme, test.object, test.validator)
				assertSerializedAdmissionResponse(t, hook, test.object)
			})
		})
	}
}

func assertSerializedAdmissionResponse(t *testing.T, hook http.Handler, object runtime.Object) {
	t.Helper()

	rawObject, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal admission object: %v", err)
	}

	const requestUID types.UID = "webhook-admission-test"
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: "AdmissionReview"},
		Request: &admissionv1.AdmissionRequest{
			UID:       requestUID,
			Kind:      metav1.GroupVersionKind{Group: GroupVersion.Group, Version: GroupVersion.Version, Kind: object.GetObjectKind().GroupVersionKind().Kind},
			Resource:  metav1.GroupVersionResource{Group: GroupVersion.Group, Version: GroupVersion.Version, Resource: "testresources"},
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: rawObject},
		},
	}
	rawReview, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal admission review: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(rawReview))
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	hook.ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("admission HTTP status = %d, want %d: %s", responseRecorder.Code, http.StatusOK, responseRecorder.Body.String())
	}

	var responseReview admissionv1.AdmissionReview
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &responseReview); err != nil {
		t.Fatalf("unmarshal admission response: %v", err)
	}
	if responseReview.Response == nil {
		t.Fatal("admission response is missing")
	}
	if responseReview.Response.UID != requestUID {
		t.Fatalf("admission response UID = %q, want %q", responseReview.Response.UID, requestUID)
	}
	if result := responseReview.Response.Result; result != nil && (result.Code == http.StatusBadRequest || result.Code == http.StatusInternalServerError) {
		t.Fatalf("admission handler failed before completing request: %d %s", result.Code, result.Message)
	}
}
