/*
Copyright 2022 The Crossplane Authors.

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

package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	yaml "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
)

// Unlike many Kubernetes projects Crossplane does not use third party testing
// libraries, per the common Go test review comments. Crossplane encourages the
// use of table driven unit tests. The tests of the crossplane-runtime project
// are representative of the testing style Crossplane encourages.
//
// https://github.com/golang/go/wiki/TestComments
// https://github.com/crossplane/crossplane/blob/master/CONTRIBUTING.md#contributing-code

func TestObserve(t *testing.T) {
	type fields struct {
		service *kopsClient
	}

	type args struct {
		ctx context.Context
		mg  resource.Managed
	}

	type want struct {
		o   managed.ExternalObservation
		err error
	}

	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		// TODO: Add test cases.
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := external{service: tc.fields.service}
			got, err := e.Observe(tc.args.ctx, tc.args.mg)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, got); diff != "" {
				t.Errorf("\n%s\ne.Observe(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

// TestInstanceGroupTaints verifies that the Taints field added to
// KopsInstanceGroupSpec both serializes into the kops InstanceGroup YAML and
// participates in drift detection.
func TestInstanceGroupTaints(t *testing.T) {
	taints := []string{"dedicated=gpu:NoSchedule"}

	// newCluster builds a Cluster with a single Node instance group carrying the
	// supplied taints. This is the desired state the provider serializes and diffs.
	newCluster := func(igTaints []string) *apisv1alpha1.Cluster {
		return &apisv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"},
			Spec: apisv1alpha1.ClusterSpec{
				ForProvider: apisv1alpha1.ClusterParameters{
					State: "s3://test-state",
					InstanceGroups: []apisv1alpha1.InstanceGroupSpec{
						{
							Name: "gpu-nodes",
							Spec: apisv1alpha1.KopsInstanceGroupSpec{
								Image:       "ami-test",
								MachineType: "g4dn.xlarge",
								MinSize:     1,
								MaxSize:     3,
								Role:        apisv1alpha1.Node,
								Subnets:     []string{"subnet-0"},
								Taints:      igTaints,
							},
						},
					},
				},
			},
		}
	}

	// seedObserved records the spec the provider would serialize as the observed
	// (atProvider) state, so diffClusterV2 reports no drift unless a field is mutated.
	seedObserved := func(cr *apisv1alpha1.Cluster) {
		_, igYamls := buildKopsYamlStructs(cr)
		observed := map[string]*apisv1alpha1.KopsInstanceGroupSpec{}
		for i := range igYamls {
			observed[igYamls[i].Metadata.Name] = igYamls[i].Spec.DeepCopy()
		}
		cr.Status.AtProvider.InstanceGroupSpecs = observed
	}

	// igDeltas filters diffClusterV2 output down to instance group changes.
	igDeltas := func(deltas []observedDelta) []observedDelta {
		out := []observedDelta{}
		for _, d := range deltas {
			if d.Resource == instanceGroupResourceDelta {
				out = append(out, d)
			}
		}
		return out
	}

	t.Run("SerializesTaintsToKopsYaml", func(t *testing.T) {
		cr := newCluster(taints)
		_, igYamls := buildKopsYamlStructs(cr)
		if len(igYamls) != 1 {
			t.Fatalf("buildKopsYamlStructs(...): expected 1 instance group yaml, got %d", len(igYamls))
		}
		if diff := cmp.Diff(taints, igYamls[0].Spec.Taints); diff != "" {
			t.Errorf("buildKopsYamlStructs(...): -want taints, +got:\n%s", diff)
		}
		out, err := yaml.Marshal(&igYamls[0])
		if err != nil {
			t.Fatalf("yaml.Marshal(...): unexpected error: %v", err)
		}
		if !strings.Contains(string(out), "taints:") || !strings.Contains(string(out), "dedicated=gpu:NoSchedule") {
			t.Errorf("marshaled instance group yaml missing taints:\n%s", string(out))
		}
	})

	t.Run("NoDriftWhenTaintsMatch", func(t *testing.T) {
		cr := newCluster(taints)
		seedObserved(cr)
		got := igDeltas((&kopsClient{}).diffClusterV2(context.Background(), cr))
		if len(got) != 0 {
			t.Errorf("diffClusterV2(...): expected no instance group deltas, got %d: %+v", len(got), got)
		}
	})

	t.Run("DriftWhenTaintsDiffer", func(t *testing.T) {
		cr := newCluster(taints)
		seedObserved(cr)
		// The cluster still desires the taint; drop it from observed state to simulate drift.
		cr.Status.AtProvider.InstanceGroupSpecs["gpu-nodes"].Taints = nil
		got := igDeltas((&kopsClient{}).diffClusterV2(context.Background(), cr))
		if len(got) != 1 {
			t.Fatalf("diffClusterV2(...): expected 1 instance group delta, got %d: %+v", len(got), got)
		}
		if got[0].Operation != updateDelta {
			t.Errorf("diffClusterV2(...): expected updateDelta, got %q", got[0].Operation)
		}
	})
}
