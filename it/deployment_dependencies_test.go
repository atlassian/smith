package it

import (
	"context"
	"testing"

	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/specchecker/builtin"
	"github.com/stretchr/testify/require"
	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
	toolswatch "k8s.io/client-go/tools/watch"
)

const (
	deploymentDependenciesConfigMapName = "map1"
	deploymentDependenciesSecretName    = "secret1"
)

func TestConfigMapAndSecretToDeploymentToBundleIndex(t *testing.T) {
	t.Parallel()

	bundle := constructDeploymentDependenciesBundle()
	SetupApp(t, bundle, false, true, assertConfigMapAndSecretToDeploymentToBundleIndex)
}

func assertConfigMapAndSecretToDeploymentToBundleIndex(ctx context.Context, t *testing.T, cfg *Config, args ...interface{}) {
	// initial create of ConfigMap and Secret
	cm, err := cfg.MainClient.CoreV1().ConfigMaps(cfg.Namespace).Create(deploymentDependenciesConfigMap())
	require.NoError(cfg.T, err)
	secret, err := cfg.MainClient.CoreV1().Secrets(cfg.Namespace).Create(deploymentDependenciesSecret())
	require.NoError(cfg.T, err)

	// Wait for stable state
	deploymentName := cfg.Bundle.Spec.Resources[0].Spec.Object.(meta_v1.Object).GetName()
	lw := cache.NewListWatchFromClient(cfg.MainClient.AppsV1().RESTClient(), "deployments", cfg.Namespace, fields.Everything())
	cond := IsPodSpecAnnotationCond(t, cfg.Namespace, deploymentName, builtin.EnvRefHashAnnotation, "97b5461821487e047e760880764dd9b31a5b7f39e8961b98b809c6d3f5b3cf3a")
	_, err = toolswatch.UntilWithSync(ctx, lw, &apps_v1.Deployment{}, nil, cond)
	require.NoError(cfg.T, err)

	// Update ConfigMap
	cm.Data["b"] = "a"
	_, err = cfg.MainClient.CoreV1().ConfigMaps(cfg.Namespace).Update(cm)
	require.NoError(cfg.T, err)

	// Wait for updated annotation
	cond = IsPodSpecAnnotationCond(t, cfg.Namespace, deploymentName, builtin.EnvRefHashAnnotation, "5fb60505d125dd42160a9ef237873345020a90e8d607861ced50a641c3a800a9")
	_, err = toolswatch.UntilWithSync(ctx, lw, &apps_v1.Deployment{}, nil, cond)
	require.NoError(cfg.T, err)

	// Update Secret
	secret.StringData = map[string]string{
		"z": "c",
	}
	_, err = cfg.MainClient.CoreV1().Secrets(cfg.Namespace).Update(secret)
	require.NoError(cfg.T, err)

	// Wait for updated annotation
	cond = IsPodSpecAnnotationCond(t, cfg.Namespace, deploymentName, builtin.EnvRefHashAnnotation, "20ed86a5106aab7e892f498f38dc1b713405d864878e6a074377fbdcba129599")
	_, err = toolswatch.UntilWithSync(ctx, lw, &apps_v1.Deployment{}, nil, cond)
	require.NoError(cfg.T, err)
}

func constructDeploymentDependenciesBundle() *smith_v1.Bundle {
	var (
		replicas                      int32 = 1
		minReadySeconds               int32 = 1
		revisionHistoryLimit          int32 = 10
		progressDeadlineSeconds       int32 = 5
		terminationGracePeriodSeconds int64 = 30
		labelMap                            = map[string]string{
			"name": string(deploymentResourceName),
		}
	)
	return &smith_v1.Bundle{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "bundle-dt",
		},
		Spec: smith_v1.BundleSpec{
			Resources: []smith_v1.Resource{
				{
					Name: deploymentResourceName,
					Spec: smith_v1.ResourceSpec{
						Object: &apps_v1.Deployment{
							TypeMeta: meta_v1.TypeMeta{
								Kind:       "Deployment",
								APIVersion: apps_v1.SchemeGroupVersion.String(),
							},
							ObjectMeta: meta_v1.ObjectMeta{
								Name: string(deploymentResourceName),
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
										Containers: []core_v1.Container{
											{
												Name:  "container1",
												Image: "roaanv/k8sti",
												EnvFrom: []core_v1.EnvFromSource{
													{
														ConfigMapRef: &core_v1.ConfigMapEnvSource{
															LocalObjectReference: core_v1.LocalObjectReference{
																Name: deploymentDependenciesConfigMapName,
															},
														},
													},
													{
														SecretRef: &core_v1.SecretEnvSource{
															LocalObjectReference: core_v1.LocalObjectReference{
																Name: deploymentDependenciesSecretName,
															},
														},
													},
												},
												ImagePullPolicy:          core_v1.PullAlways,
												TerminationMessagePath:   "/dev/termination-log",
												TerminationMessagePolicy: core_v1.TerminationMessageReadFile,
											},
										},
										DNSPolicy:                     core_v1.DNSClusterFirst,
										RestartPolicy:                 core_v1.RestartPolicyAlways,
										SchedulerName:                 "default-scheduler",
										SecurityContext:               &core_v1.PodSecurityContext{},
										TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
									},
								},
								MinReadySeconds:         minReadySeconds,
								ProgressDeadlineSeconds: &progressDeadlineSeconds,
								RevisionHistoryLimit:    &revisionHistoryLimit,

								Strategy: apps_v1.DeploymentStrategy{
									Type: apps_v1.RollingUpdateDeploymentStrategyType,
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
				},
			},
		},
	}
}

func deploymentDependenciesConfigMap() *core_v1.ConfigMap {
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: deploymentDependenciesConfigMapName,
		},
		Data: map[string]string{
			"a": "b",
		},
	}
}

func deploymentDependenciesSecret() *core_v1.Secret {
	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: deploymentDependenciesSecretName,
		},
		StringData: map[string]string{
			"x": "b",
		},
	}
}
