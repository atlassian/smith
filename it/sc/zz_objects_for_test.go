package sc

import (
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	serviceInstance1name  = "instance1"
	serviceBinding1name   = "binding1"
	serviceBinding1secret = "binding1secret"
	secret1name           = "secret1"
	secret1credentialsKey = "credentials"
	secret2name           = "secret2"
)

func serviceInstance1() *sc_v1b1.ServiceInstance {
	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceInstance1name,
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassExternalName: "user-provided-service",
				ClusterServicePlanExternalName:  "default",
			},
			Parameters: &runtime.RawExtension{
				Raw: []byte(`{
	"` + secret1credentialsKey + `": {
		"token": "token"
	}
}`),
			},
		},
	}
}

func serviceInstance1withParametersFrom() *sc_v1b1.ServiceInstance {
	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceInstance1name,
		},
		Spec: sc_v1b1.ServiceInstanceSpec{
			PlanReference: sc_v1b1.PlanReference{
				ClusterServiceClassExternalName: "user-provided-service",
				ClusterServicePlanExternalName:  "default",
			},
			ParametersFrom: []sc_v1b1.ParametersFromSource{
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: secret1name,
						Key:  secret1credentialsKey,
					},
				},
			},
		},
	}
}

func serviceBinding1() *sc_v1b1.ServiceBinding {
	return &sc_v1b1.ServiceBinding{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceBinding",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceBinding1name,
		},
		Spec: sc_v1b1.ServiceBindingSpec{
			ServiceInstanceRef: sc_v1b1.LocalObjectReference{
				Name: serviceInstance1name,
			},
			SecretName: serviceBinding1secret,
		},
	}
}

func serviceBinding1withParametersFrom() *sc_v1b1.ServiceBinding {
	return &sc_v1b1.ServiceBinding{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceBinding",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: serviceBinding1name,
		},
		Spec: sc_v1b1.ServiceBindingSpec{
			ServiceInstanceRef: sc_v1b1.LocalObjectReference{
				Name: serviceInstance1name,
			},
			ParametersFrom: []sc_v1b1.ParametersFromSource{
				{
					SecretKeyRef: &sc_v1b1.SecretKeyReference{
						Name: secret1name,
						Key:  secret1credentialsKey,
					},
				},
			},
			SecretName: serviceBinding1secret,
		},
	}
}

func secret1() *core_v1.Secret {
	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: secret1name,
		},
		StringData: map[string]string{
			secret1credentialsKey: `{"token": "token"}`,
		},
	}
}

func secret2() *core_v1.Secret {
	return &core_v1.Secret{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Secret",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name: secret2name,
		},
		StringData: map[string]string{
			"y": "z",
		},
	}
}
