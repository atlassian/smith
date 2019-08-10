package bundlec_test

import (
	"testing"

	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	"github.com/atlassian/smith/pkg/plugin"
	sc_v1b1 "github.com/kubernetes-sigs/service-catalog/pkg/apis/servicecatalog/v1beta1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type testingPlugin interface {
	plugin.Plugin
	WasInvoked() bool
}

func configMapWithDependenciesPlugin(expectBinding, expectSleeper bool) func(t *testing.T) testingPlugin {
	return func(t *testing.T) testingPlugin {
		return &configMapWithDeps{
			t:             t,
			expectBinding: expectBinding,
			expectSleeper: expectSleeper,
		}
	}
}

type configMapWithDeps struct {
	t             *testing.T
	expectBinding bool
	expectSleeper bool
	wasInvoked    bool
}

func (p *configMapWithDeps) WasInvoked() bool {
	return p.wasInvoked
}

func (p *configMapWithDeps) Describe() *plugin.Description {
	return &plugin.Description{
		Name: pluginConfigMapWithDeps,
		GVK:  core_v1.SchemeGroupVersion.WithKind("ConfigMap"),
		SpecSchema: []byte(`{
			"type": "object",
			"properties": {
				"p1": {
					"type": "string"
				},
				"p2": {
					"type": "string"
				}
			}
		}`),
	}
}

func (p *configMapWithDeps) Process(pluginSpec map[string]interface{}, context *plugin.Context) plugin.ProcessResult {
	p.wasInvoked = true
	failed := p.t.Failed()

	assert.Equal(p.t, testNamespace, context.Namespace)

	actualShouldExist, _ := pluginSpec["actualShouldExist"].(bool)
	delete(pluginSpec, "actualShouldExist")
	assert.Equal(p.t, map[string]interface{}{"p1": "v1", "p2": sb1}, pluginSpec)

	if actualShouldExist {
		assert.IsType(p.t, &core_v1.ConfigMap{}, context.Actual)
	} else {
		assert.Nil(p.t, context.Actual)
	}

	bindingDep, ok := context.Dependencies[resSb1]
	if p.expectBinding && assert.True(p.t, ok) {
		// Actual
		if assert.IsType(p.t, &sc_v1b1.ServiceBinding{}, bindingDep.Actual) {
			b := bindingDep.Actual.(*sc_v1b1.ServiceBinding)
			assert.Equal(p.t, sb1, b.Name)
			assert.Equal(p.t, testNamespace, b.Namespace)
			assert.Equal(p.t, sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding"), b.GroupVersionKind())
		}
		// Outputs
		if assert.Len(p.t, bindingDep.Outputs, 1) {
			secret := bindingDep.Outputs[0]
			if assert.IsType(p.t, &core_v1.Secret{}, secret) {
				s := secret.(*core_v1.Secret)
				assert.Equal(p.t, s1, s.Name)
				assert.Equal(p.t, testNamespace, s.Namespace)
				assert.Equal(p.t, core_v1.SchemeGroupVersion.WithKind("Secret"), s.GroupVersionKind())
			}
		}
		// Aux
		if assert.Len(p.t, bindingDep.Auxiliary, 1) {
			svcInst := bindingDep.Auxiliary[0]
			if assert.IsType(p.t, &sc_v1b1.ServiceInstance{}, svcInst) {
				inst := svcInst.(*sc_v1b1.ServiceInstance)
				assert.Equal(p.t, si1, inst.Name)
				assert.Equal(p.t, testNamespace, inst.Namespace)
				assert.Equal(p.t, sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"), inst.GroupVersionKind())
			}
		}
	}
	sleeperDep, ok := context.Dependencies[resSleeper1]
	if p.expectSleeper && assert.True(p.t, ok) {
		// Actual
		if assert.IsType(p.t, &unstructured.Unstructured{}, sleeperDep.Actual) {
			s := sleeperDep.Actual.(*unstructured.Unstructured)
			assert.Equal(p.t, sleeper1, s.GetName())
			assert.Equal(p.t, testNamespace, s.GetNamespace())
			assert.Equal(p.t, sleeper_v1.SleeperGVK, s.GroupVersionKind())
		}
		// Outputs
		assert.Empty(p.t, sleeperDep.Outputs)
		// Aux
		assert.Empty(p.t, sleeperDep.Auxiliary)
	}

	if !failed && p.t.Failed() { // one of the assertions failed and it was the first failure in the test
		return &plugin.ProcessResultFailure{
			Error: errors.New("plugin failed BOOM!"),
		}
	}

	return &plugin.ProcessResultSuccess{
		Object: &core_v1.ConfigMap{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
		},
	}
}

func simpleConfigMapPlugin(t *testing.T) testingPlugin {
	return &simpleConfigMap{
		t: t,
	}
}

type simpleConfigMap struct {
	t          *testing.T
	wasInvoked bool
}

func (p *simpleConfigMap) WasInvoked() bool {
	return p.wasInvoked
}

func (p *simpleConfigMap) Describe() *plugin.Description {
	return &plugin.Description{
		Name: pluginSimpleConfigMap,
		GVK:  core_v1.SchemeGroupVersion.WithKind("ConfigMap"),
	}
}

func (p *simpleConfigMap) Process(pluginSpec map[string]interface{}, context *plugin.Context) plugin.ProcessResult {
	p.wasInvoked = true
	failed := p.t.Failed()

	assert.Equal(p.t, testNamespace, context.Namespace)

	actualShouldExist, _ := pluginSpec["actualShouldExist"].(bool)

	if actualShouldExist {
		assert.IsType(p.t, &core_v1.ConfigMap{}, context.Actual)
	} else {
		assert.Nil(p.t, context.Actual)
	}

	if !failed && p.t.Failed() { // one of the assertions failed and it was the first failure in the test
		return &plugin.ProcessResultFailure{
			Error: errors.New("plugin failed BOOM!"),
		}
	}

	return &plugin.ProcessResultSuccess{
		Object: &core_v1.ConfigMap{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
		},
	}
}

func mockConfigMapPlugin(configMap *core_v1.ConfigMap) func(*testing.T) testingPlugin {
	return func(t *testing.T) testingPlugin {
		return &mockConfigMap{
			t:         t,
			configMap: configMap,
		}
	}
}

type mockConfigMap struct {
	t          *testing.T
	wasInvoked bool
	configMap  *core_v1.ConfigMap
}

func (p *mockConfigMap) WasInvoked() bool {
	return p.wasInvoked
}

func (p *mockConfigMap) Describe() *plugin.Description {
	return &plugin.Description{
		Name: pluginMockConfigMap,
		GVK:  core_v1.SchemeGroupVersion.WithKind("ConfigMap"),
	}
}

func (p *mockConfigMap) Process(pluginSpec map[string]interface{}, context *plugin.Context) plugin.ProcessResult {
	p.wasInvoked = true
	failed := p.t.Failed()

	assert.Equal(p.t, testNamespace, context.Namespace)

	actualShouldExist, _ := pluginSpec["actualShouldExist"].(bool)

	if actualShouldExist {
		assert.IsType(p.t, &core_v1.ConfigMap{}, context.Actual)
	} else {
		assert.Nil(p.t, context.Actual)
	}

	if !failed && p.t.Failed() { // one of the assertions failed and it was the first failure in the test
		return &plugin.ProcessResultFailure{
			Error: errors.New("plugin failed BOOM!"),
		}
	}

	return &plugin.ProcessResultSuccess{
		Object: p.configMap,
	}
}

func newFailingPlugin(t *testing.T) testingPlugin {
	return &failingPluginStruct{
		t: t,
	}
}

type failingPluginStruct struct {
	t          *testing.T
	wasInvoked bool
}

func (p *failingPluginStruct) WasInvoked() bool {
	return p.wasInvoked
}

func (p *failingPluginStruct) Describe() *plugin.Description {
	return &plugin.Description{
		Name: pluginFailing,
		GVK:  core_v1.SchemeGroupVersion.WithKind("ConfigMap"),
	}
}

func (p *failingPluginStruct) Process(pluginSpec map[string]interface{}, context *plugin.Context) plugin.ProcessResult {
	p.wasInvoked = true
	return &plugin.ProcessResultFailure{
		Error: errors.New("plugin failed as it should. BOOM!"),
	}
}
