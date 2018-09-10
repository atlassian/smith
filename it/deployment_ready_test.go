package it

import (
	"context"
	"strconv"
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/atlassian/smith"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	"github.com/stretchr/testify/assert"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func constructBundle(t *testing.T, progressDeadlineSeconds int32, containerParams ...string) *smith_v1.Bundle {
	t.Parallel()

	// This is a hack for now until someone can help
	// This is "set" in the deploymentCleanup but that is AFTER the object was created
	// So this never really works since it should be set before the object was created
	// in processResource "createOrUpdate"
	const LastAppliedReplicasAnnotation string = smith.Domain + "/LastAppliedReplicas"

	resourceName := smith_v1.ResourceName("deployment-ready-test")

	labelMap := map[string]string{
		"name": string(resourceName),
	}
	replicas := int32(2)
	containers := []core_v1.Container{
		core_v1.Container{
			Name:                     "container1",
			Image:                    "roaanv/k8sti",
			Args:                     containerParams,
			ImagePullPolicy:          "Always",
			TerminationMessagePath:   "/dev/termination-log",
			TerminationMessagePolicy: core_v1.TerminationMessagePolicy("File"),
			LivenessProbe: &core_v1.Probe{
				InitialDelaySeconds: 2,
				PeriodSeconds:       2,
				FailureThreshold:    3,
				SuccessThreshold:    1,
				TimeoutSeconds:      1,
				Handler: core_v1.Handler{
					HTTPGet: &core_v1.HTTPGetAction{
						Port:   intstr.FromInt(8080),
						Path:   "/live",
						Scheme: "HTTP",
					},
				},
			},
			ReadinessProbe: &core_v1.Probe{
				InitialDelaySeconds: 2,
				PeriodSeconds:       2,
				FailureThreshold:    3,
				SuccessThreshold:    1,
				TimeoutSeconds:      1,
				Handler: core_v1.Handler{
					HTTPGet: &core_v1.HTTPGetAction{
						Port:   intstr.FromInt(8080),
						Path:   "/ready",
						Scheme: "HTTP",
					},
				},
			},
		},
	}
	var minReadySeconds int32 = 1
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
					Name:        string(resourceName),
					Annotations: map[string]string{LastAppliedReplicasAnnotation: strconv.Itoa(int(replicas))},
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
	bundle := constructBundle(t, 10, "-l", "0", "-r", "0")
	SetupApp(t, bundle, false, true, assertSuccess)
}

func TestDeploymentNeverReady(t *testing.T) {
	bundle := constructBundle(t, 10, "-l", "20", "-r", "20")
	SetupApp(t, bundle, false, true, assertDeadlineExceeded)
}

func assertSuccess(ctx context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	bundleRes := cfg.AwaitBundleCondition(ctx, IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleReady, cond_v1.ConditionTrue))

	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleReady, cond_v1.ConditionTrue)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleInProgress, cond_v1.ConditionFalse)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleError, cond_v1.ConditionFalse)
}

func assertDeadlineExceeded(ctx context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	bundleRes := cfg.AwaitBundleCondition(ctx, IsBundleStatusCond(cfg.Namespace, cfg.Bundle.Name, smith_v1.BundleError, cond_v1.ConditionTrue))

	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleReady, cond_v1.ConditionFalse)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleInProgress, cond_v1.ConditionFalse)
	smith_testing.AssertCondition(cfg.T, bundleRes, smith_v1.BundleError, cond_v1.ConditionTrue)

	resourceStatus := bundleRes.Status.ResourceStatuses[0]
	_, cond := cond_v1.FindCondition(resourceStatus.Conditions, smith_v1.BundleError)
	assert.Contains(t, cond.Message, "exceeded its progress deadline")
}
