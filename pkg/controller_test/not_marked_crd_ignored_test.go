package controller_test

import (
	"testing"

	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/controller"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Should not setup watch for a CRD that does not have smith.CrdSupportEnabled annotation set to true
func TestNotMarkedCrdIgnored(t *testing.T) {
	t.Parallel()
	t.Run("set to false", func(t *testing.T) {
		t.Parallel()
		sleeperCrd := SleeperCrdWithStatus()
		sleeperCrd.Annotations[smith.CrdSupportEnabled] = "false"
		tc := testCase{
			crdClientObjects: []runtime.Object{
				sleeperCrd,
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:       bundle1,
					Namespace:  testNamespace,
					UID:        bundle1uid,
					Finalizers: []string{controller.FinalizerDeleteResources},
				},
			},
			namespace: testNamespace,
		}
		tc.run(t)
	})
	t.Run("not set", func(t *testing.T) {
		t.Parallel()
		sleeperCrd := SleeperCrdWithStatus()
		delete(sleeperCrd.Annotations, smith.CrdSupportEnabled)
		tc := testCase{
			crdClientObjects: []runtime.Object{
				sleeperCrd,
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:       bundle1,
					Namespace:  testNamespace,
					UID:        bundle1uid,
					Finalizers: []string{controller.FinalizerDeleteResources},
				},
			},
			namespace: testNamespace,
		}
		tc.run(t)
	})
}
