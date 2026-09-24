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

package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

const (
	providerContract = "v1beta1"
	contractLabelKey = "cluster.x-k8s.io/v1beta1"
)

func TestProviderContractSignalsStayAligned(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")

	var metadata struct {
		ReleaseSeries []struct {
			Major    int    `yaml:"major"`
			Minor    int    `yaml:"minor"`
			Contract string `yaml:"contract"`
		} `yaml:"releaseSeries"`
	}
	readYAML(t, filepath.Join(repositoryRoot, "metadata.yaml"), &metadata)
	if len(metadata.ReleaseSeries) == 0 {
		t.Fatal("root metadata has no release series")
	}
	for _, series := range metadata.ReleaseSeries {
		if series.Contract != providerContract {
			t.Fatalf("root metadata release series %d.%d has contract %q, want %q", series.Major, series.Minor, series.Contract, providerContract)
		}
	}

	var kustomization struct {
		CommonLabels map[string]string `yaml:"commonLabels"`
		Resources    []string          `yaml:"resources"`
	}
	readYAML(t, filepath.Join(repositoryRoot, "config", "crd", "kustomization.yaml"), &kustomization)
	if got := kustomization.CommonLabels[contractLabelKey]; got != "v1beta1_v1beta2" {
		t.Fatalf("CRD contract label %q = %q, want %q", contractLabelKey, got, "v1beta1_v1beta2")
	}
	if len(kustomization.Resources) != 14 {
		t.Fatalf("CRD kustomization has %d resources, want 14 contract-labeled CRDs", len(kustomization.Resources))
	}

	var e2eConfig struct {
		Providers []struct {
			Name     string `yaml:"name"`
			Type     string `yaml:"type"`
			Versions []struct {
				Contract string `yaml:"contract"`
				Files    []struct {
					SourcePath string `yaml:"sourcePath"`
				} `yaml:"files"`
			} `yaml:"versions"`
		} `yaml:"providers"`
	}
	readYAML(t, filepath.Join(repositoryRoot, "test", "e2e", "config", "e2e_conf.yaml"), &e2eConfig)

	foundOCIProvider := false
	for _, provider := range e2eConfig.Providers {
		if provider.Name != "oci" || provider.Type != "InfrastructureProvider" {
			continue
		}
		foundOCIProvider = true
		for _, version := range provider.Versions {
			if version.Contract != providerContract {
				t.Fatalf("OCI E2E provider contract = %q, want %q", version.Contract, providerContract)
			}
			foundMetadata := false
			for _, file := range version.Files {
				if filepath.Base(file.SourcePath) != "metadata.yaml" {
					continue
				}
				foundMetadata = true
				if file.SourcePath != "../data/infrastructure-oci/v1beta1/metadata.yaml" {
					t.Fatalf("OCI E2E metadata source = %q, want the v1beta1 fixture", file.SourcePath)
				}
			}
			if !foundMetadata {
				t.Fatal("OCI E2E provider has no metadata fixture")
			}
		}
	}
	if !foundOCIProvider {
		t.Fatal("OCI infrastructure provider is missing from E2E configuration")
	}
}

func readYAML(t *testing.T, path string, out interface{}) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
