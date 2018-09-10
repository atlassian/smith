package ready

import (
	"context"
	"testing"

	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/atlassian/smith/it"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"

	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	apps_v1 "k8s.io/api/apps/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createBundle(t *testing.T, containerParams ...string) *smith_v1.Bundle {
	t.Parallel()
	resourceName := smith_v1.ResourceName("deployment-ready")

	labelMap := map[string]string{
		"name": string(resourceName),
	}
	replicas := int32(2)
	containers := []core_v1.Container{
		core_v1.Container{
			Name:                     "container1",
			Image:                    "roaanv/k8sti",
			Command:                  containerParams,
			ImagePullPolicy:          "Always",
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: core_v1.TerminationMessagePolicy("File"),
		},
	}
	var minReadySeconds int32 = 1
	var progressDeadlineSeconds int32 = 2
	var revisionHistoryLimit int32 = 10
	var terminationGracePeriodSeconds int64 = 30

	deployment := smith_v1.Resource{
		Name: resourceName,
		Spec: smith_v1.ResourceSpec{
			Object: &apps_v1.Deployment{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "Deployment",
					APIVersion: apps_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: string(resourceName),
				},
				Spec: apps_v1.DeploymentSpec{
					Selector: &meta_v1.LabelSelector{
						MatchLabels: labelMap,
					},
					Replicas: &replicas,
					Template: core_v1.PodTemplateSpec{
						ObjectMeta: meta_v1.ObjectMeta{
							Labels: labelMap,
						},
						Spec: core_v1.PodSpec{
							Containers:                    containers,
							DNSPolicy:                     "ClusterFirst",
							RestartPolicy:                 "Always",
							SchedulerName:                 "default-scheduler",
							SecurityContext:               &core_v1.PodSecurityContext{},
							TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
						},
					},
					MinReadySeconds:         minReadySeconds,
					ProgressDeadlineSeconds: &progressDeadlineSeconds,
					RevisionHistoryLimit:    &revisionHistoryLimit,

					Strategy: apps_v1.DeploymentStrategy{
						Type: "RollingUpdate",
						RollingUpdate: &apps_v1.RollingUpdateDeployment{
							MaxUnavailable: &intstr.IntOrString{
								Type:   intstr.String,
								StrVal: "25%",
							},
							MaxSurge: &intstr.IntOrString{
								Type:   intstr.String,
								StrVal: "25%",
							},
						},
					},
				},
			},
		},
	}

	bundle := &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-dt",
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{deployment},
		},
	}

	return bundle
}

func TestDeploymentReady(t *testing.T) {
	//bundle := createBundle([]string{"-l", "0", "-r", "0"})
	bundle := createBundle(t, "-l", "0", "-r", "0")
	it.SetupApp(t, bundle, false, true, testDeploymentDetection)
}

func TestDeploymentNeverReady(t *testing.T) {
	//bundle := createBundle([]string{"-l", "0", "-r", "0"})
	bundle := createBundle(t, "-l", "120", "-r", "120")
	it.SetupApp(t, bundle, false, true, testDeploymentDetection)
}

func testDeploymentDetection(ctx context.Context, t *testing.T, cfg *it.Config, args ...interface{}) {
	bundleRes := cfg.AwaitBundleCondition(it.IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleReady, smith_v1.ConditionTrue))

	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleReady, smith_v1.ConditionTrue)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleInProgress, smith_v1.ConditionFalse)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleError, smith_v1.ConditionFalse)
}
