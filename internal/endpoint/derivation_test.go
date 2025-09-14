/*
Copyright 2025.

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

package endpoint

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestEndpointDeriver_DeriveEndpoint(t *testing.T) {
	scheme := runtime.NewScheme()
	err := scorev1b1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	deriver := NewEndpointDeriver(client)

	tests := []struct {
		name     string
		workload *scorev1b1.Workload
		plan     *scorev1b1.WorkloadPlan
		want     string
		wantErr  bool
	}{
		{
			name: "no plan returns empty endpoint",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			},
			plan:    nil,
			want:    "",
			wantErr: false,
		},
		{
			name: "single http port",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{
								Port: 8080,
							},
						},
					},
				},
			},
			plan: &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			},
			want:    "http://test-workload.test-ns.svc.cluster.local:8080",
			wantErr: false,
		},
		{
			name: "https port prioritized over http",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{
								Port: 8080,
							},
							{
								Port: 8443,
							},
						},
					},
				},
			},
			plan: &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			},
			want:    "https://test-workload.test-ns.svc.cluster.local:8443",
			wantErr: false,
		},
		{
			name: "https port prioritized over non-standard port",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{
								Port: 8443,
							},
							{
								Port: 3000,
							},
							{
								Port: 8080,
							},
						},
					},
				},
			},
			plan: &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			},
			want:    "https://test-workload.test-ns.svc.cluster.local:8443",
			wantErr: false,
		},
		{
			name: "standard port 80 omitted",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{
								Port: 80,
							},
						},
					},
				},
			},
			plan: &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			},
			want:    "http://test-workload.test-ns.svc.cluster.local",
			wantErr: false,
		},
		{
			name: "standard port 443 omitted",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{
								Port: 443,
							},
						},
					},
				},
			},
			plan: &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			},
			want:    "https://test-workload.test-ns.svc.cluster.local",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deriver.DeriveEndpoint(context.TODO(), tt.workload, tt.plan)
			if (err != nil) != tt.wantErr {
				t.Errorf("EndpointDeriver.DeriveEndpoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("EndpointDeriver.DeriveEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointDeriver_prioritizePortsByCharacteristics(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	deriver := NewEndpointDeriver(client)

	tests := []struct {
		name  string
		ports []scorev1b1.ServicePort
		want  []int32 // Expected port numbers in priority order
	}{
		{
			name: "https port has highest priority",
			ports: []scorev1b1.ServicePort{
				{Port: 3000},
				{Port: 8443},
				{Port: 8080},
			},
			want: []int32{8443, 8080, 3000},
		},
		{
			name: "http port over unknown port",
			ports: []scorev1b1.ServicePort{
				{Port: 3000},
				{Port: 8080},
			},
			want: []int32{8080, 3000},
		},
		{
			name: "standard https port prioritized",
			ports: []scorev1b1.ServicePort{
				{Port: 8080},
				{Port: 443},
			},
			want: []int32{443, 8080},
		},
		{
			name: "standard http port prioritized",
			ports: []scorev1b1.ServicePort{
				{Port: 3000},
				{Port: 80},
			},
			want: []int32{80, 3000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriver.prioritizePortsByCharacteristics(tt.ports)
			if len(got) != len(tt.want) {
				t.Errorf("prioritizePortsByCharacteristics() returned %d ports, want %d", len(got), len(tt.want))
				return
			}

			for i, expectedPort := range tt.want {
				if got[i].Port != expectedPort {
					t.Errorf("prioritizePortsByCharacteristics()[%d].Port = %v, want %v", i, got[i].Port, expectedPort)
				}
			}
		})
	}
}

func TestEndpointDeriver_isHTTPSPort(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	deriver := NewEndpointDeriver(client)

	tests := []struct {
		name    string
		portNum int32
		want    bool
	}{
		{"port 443", 443, true},
		{"port 8443", 8443, true},
		{"port 80", 80, false},
		{"port 8080", 8080, false},
		{"random port", 3000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriver.isHTTPSPort(tt.portNum); got != tt.want {
				t.Errorf("isHTTPSPort(%v) = %v, want %v", tt.portNum, got, tt.want)
			}
		})
	}
}

func TestEndpointDeriver_isHTTPPort(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	deriver := NewEndpointDeriver(client)

	tests := []struct {
		name    string
		portNum int32
		want    bool
	}{
		{"port 80", 80, true},
		{"port 8080", 8080, true},
		{"port 443", 443, false},
		{"port 8443", 8443, false},
		{"random port", 3000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriver.isHTTPPort(tt.portNum); got != tt.want {
				t.Errorf("isHTTPPort(%v) = %v, want %v", tt.portNum, got, tt.want)
			}
		})
	}
}

func TestEndpointDeriver_generateHostname(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	deriver := NewEndpointDeriver(client)

	tests := []struct {
		name     string
		workload *scorev1b1.Workload
		want     string
	}{
		{
			name: "basic hostname generation",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "production",
				},
			},
			want: "my-app.production.svc.cluster.local",
		},
		{
			name: "default namespace",
			workload: &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			},
			want: "test-service.default.svc.cluster.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriver.generateHostname(tt.workload); got != tt.want {
				t.Errorf("generateHostname() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointDeriver_isStandardPort(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	deriver := NewEndpointDeriver(client)

	tests := []struct {
		name   string
		scheme string
		port   int32
		want   bool
	}{
		{"HTTP port 80", "http", 80, true},
		{"HTTPS port 443", "https", 443, true},
		{"HTTP port 8080", "http", 8080, false},
		{"HTTPS port 8443", "https", 8443, false},
		{"HTTP port 443", "http", 443, false},
		{"HTTPS port 80", "https", 80, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriver.isStandardPort(tt.scheme, tt.port); got != tt.want {
				t.Errorf("isStandardPort(%v, %v) = %v, want %v", tt.scheme, tt.port, got, tt.want)
			}
		})
	}
}
