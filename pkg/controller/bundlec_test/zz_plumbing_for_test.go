package bundlec_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/smith"
	"github.com/atlassian/smith/cmd/smith/app"
	"github.com/atlassian/smith/examples/sleeper"
	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithFake "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/fake"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/controller"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/util"
	smith_testing "github.com/atlassian/smith/pkg/util/testing"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	scFake "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiExtFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	mainFake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kube_testing "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type reaction struct {
	verb     string
	resource string
	reactor  func(*testing.T) kube_testing.ReactionFunc
}

type testCase struct {
	logger              *zap.Logger
	mainClientObjects   []runtime.Object
	mainReactors        []reaction
	smithClientObjects  []runtime.Object
	smithReactors       []reaction
	apiExtClientObjects []runtime.Object
	apiExtReactors      []reaction
	scClientObjects     []runtime.Object
	scReactors          []reaction
	bundle              *smith_v1.Bundle
	namespace           string

	expectedActions        sets.String
	enableServiceCatalog   bool
	testHandler            fakeActionHandler
	test                   func(*testing.T, context.Context, *bundlec.Controller, *testCase)
	plugins                map[smith_v1.PluginName]func(*testing.T) testingPlugin
	pluginsShouldBeInvoked sets.String
	testTimeout            time.Duration

	mainFake           *kube_testing.Fake
	smithFake          *kube_testing.Fake
	apiExtFake         *kube_testing.Fake
	scFake             *kube_testing.Fake
	pluginsConstructed []testingPlugin
}

const (
	testNamespace = "test-namespace"

	resSb1 = "resSb1"
	sb1    = "sb1"
	sb1uid = "sb1-uid"

	resSi1 = "resSi1"
	si1    = "si1"
	si1uid = "si1-uid"
	resSi2 = "resSi2"
	si2    = "si2"
	si2uid = "si2-uid"

	s1    = "s1"
	s1uid = "s1-uid"

	m1 = "m1"

	resP1 = "resP1"

	resSleeper1 = "resSleeper1"
	sleeper1    = "sleeper1"
	sleeper1uid = "sleeper1-uid"

	bundle1    = "bundle1"
	bundle1uid = "bundle1-uid"

	resMapNeedsAnUpdate = "res-map-needs-update"
	mapNeedsAnUpdate    = "map-needs-update"
	mapNeedsAnUpdateUid = "map-needs-update-uid"

	mapNeedsDelete    = "map-not-in-the-bundle-anymore-needs-delete"
	mapNeedsDeleteUid = "map-needs-delete-uid"

	pluginSimpleConfigMap   = "simpleConfigMap"
	pluginConfigMapWithDeps = "configMapWithDeps"

	serviceClassName         = "uid-1"
	serviceClassExternalName = "database"
	servicePlanName          = "uid-2"
	servicePlanExternalName  = "default"
)

var (
	serviceInstanceSpec = sc_v1b1.ServiceInstanceSpec{
		PlanReference: sc_v1b1.PlanReference{
			ClusterServiceClassExternalName: serviceClassExternalName,
			ClusterServicePlanExternalName:  servicePlanExternalName,
		},
	}
)

func (tc *testCase) run(t *testing.T) {
	mainClient := mainFake.NewSimpleClientset(tc.mainClientObjects...)
	tc.mainFake = &mainClient.Fake
	for _, reactor := range tc.mainReactors {
		mainClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	if tc.bundle != nil {
		convertBundleResourcesToUnstrucutred(t, tc.bundle, tc.enableServiceCatalog)
		tc.smithClientObjects = append(tc.smithClientObjects, tc.bundle)
	}
	smithClient := smithFake.NewSimpleClientset(tc.smithClientObjects...)
	tc.smithFake = &smithClient.Fake
	for _, reactor := range tc.smithReactors {
		smithClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	apiExtClient := apiExtFake.NewSimpleClientset(tc.apiExtClientObjects...)
	tc.apiExtFake = &apiExtClient.Fake
	for _, reactor := range tc.apiExtReactors {
		apiExtClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}

	var scClient scClientset.Interface
	if tc.enableServiceCatalog {
		scClientFake := scFake.NewSimpleClientset(append(
			tc.scClientObjects,
			[]runtime.Object{
				&sc_v1b1.ClusterServiceClass{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ClusterServiceClass",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: serviceClassName,
					},
					Spec: sc_v1b1.ClusterServiceClassSpec{
						CommonServiceClassSpec: sc_v1b1.CommonServiceClassSpec{
							ExternalName: serviceClassExternalName,
							ExternalID:   serviceClassName,
						},
					},
				},
				&sc_v1b1.ClusterServicePlan{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ClusterServicePlan",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: servicePlanName,
					},
					Spec: sc_v1b1.ClusterServicePlanSpec{
						ClusterServiceClassRef: sc_v1b1.ClusterObjectReference{
							Name: serviceClassName,
						},
						CommonServicePlanSpec: sc_v1b1.CommonServicePlanSpec{
							ExternalName: servicePlanExternalName,
							ExternalID:   servicePlanName,
							ServiceInstanceCreateParameterSchema: &runtime.RawExtension{Raw: []byte(`
{"type": "object", "properties": {"testSchema": {"type": "boolean"}}}
`)},
						},
					},
				},
			}...)...,
		)

		tc.scFake = &scClientFake.Fake
		scClient = scClientFake
		for _, reactor := range tc.scReactors {
			scClientFake.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
		}
	}

	plugins := make([]plugin.NewFunc, 0, len(tc.plugins))
	for _, factory := range tc.plugins {
		pluginInstance := factory(t)
		plugins = append(plugins, func() (plugin.Plugin, error) { return pluginInstance, nil })
		tc.pluginsConstructed = append(tc.pluginsConstructed, pluginInstance)
	}
	scheme, err := app.FullScheme(tc.enableServiceCatalog)
	require.NoError(t, err)

	for _, object := range tc.apiExtClientObjects {
		crd := object.(*apiext_v1b1.CustomResourceDefinition)
		scheme.AddKnownTypeWithName(schema.GroupVersionKind{
			Group:   crd.Spec.Group,
			Version: crd.Spec.Version,
			Kind:    crd.Spec.Names.Kind,
			// obj: unstructured.Unstructured
			// is here _only_ to keep rest scheme happy, we do not currently use scheme to deserialize
		}, &unstructured.Unstructured{})
	}
	restMapper := restMapperFromScheme(scheme)

	tc.logger = smith_testing.DevelopmentLogger(t)
	defer tc.logger.Sync()

	// Look at k8s.io/kubernetes/pkg/controller/garbagecollector/garbagecollector_test.go for inspiration
	srv, clientConfig := testServerAndClientConfig(&tc.testHandler)
	defer srv.Close()

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	stage := stgr.NextStage()

	// Controller
	config := &controller.Config{
		Logger:       tc.logger,
		Namespace:    tc.namespace,
		MainClient:   mainClient,
		ApiExtClient: apiExtClient,
		ScClient:     scClient,
		SmithClient:  smithClient,
		SmartClient: &smart.DynamicClient{
			ClientPool: dynamic.NewClientPool(clientConfig, restMapper, dynamic.LegacyAPIPathResolverFunc),
			Mapper:     restMapper,
		},
	}
	bundleConstr := &app.BundleControllerConstructor{
		Plugins:               plugins,
		ServiceCatalogSupport: tc.enableServiceCatalog,
	}
	generic, err := controller.NewGeneric(config, tc.logger,
		workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "multiqueue"),
		2, bundleConstr)
	require.NoError(t, err)
	cntrlr := generic.Controllers[smith_v1.BundleGVK].(*bundlec.Controller)

	// Start all informers then wait on them
	for _, inf := range generic.Informers {
		stage.StartWithChannel(inf.Run) // Must be after AddInformer()
	}
	for _, inf := range generic.Informers {
		require.True(t, cache.WaitForCacheSync(ctx.Done(), inf.HasSynced))
	}

	defer tc.verifyActions(t)
	defer tc.verifyPlugins(t)

	if tc.testTimeout != 0 {
		testCtx, testCancel := context.WithTimeout(ctx, tc.testTimeout)
		defer testCancel()
		ctx = testCtx
	}

	if tc.test == nil {
		tc.defaultTest(t, ctx, cntrlr)
	} else {
		tc.test(t, ctx, cntrlr, tc)
	}
}

func (tc *testCase) defaultTest(t *testing.T, ctx context.Context, cntrlr *bundlec.Controller) {
	require.NotNil(t, tc.bundle)
	_, err := cntrlr.ProcessBundle(tc.logger, tc.bundle)
	assert.NoError(t, err)
}

func (tc *testCase) verifyActions(t *testing.T) {
	actualActions := sets.NewString()
	for _, actualAction := range tc.testHandler.getActions() {
		actualActions.Insert(actualAction.String())
	}
	missingActions := tc.expectedActions.Difference(actualActions)
	unexpectedActions := actualActions.Difference(tc.expectedActions)
	assert.Empty(t, missingActions, "expected but was not observed")
	assert.Empty(t, unexpectedActions, "observed but was not expected")
}

func (tc *testCase) verifyPlugins(t *testing.T) {
	invokedPlugins := sets.NewString()
	for _, constructedPlugin := range tc.pluginsConstructed {
		if constructedPlugin.WasInvoked() {
			invokedPlugins.Insert(string(constructedPlugin.Describe().Name))
		}
	}
	missingPlugins := tc.pluginsShouldBeInvoked.Difference(invokedPlugins)
	unexpectedInvocations := invokedPlugins.Difference(tc.pluginsShouldBeInvoked)
	assert.Empty(t, missingPlugins, "expected plugin invocations that were not observed")
	assert.Empty(t, unexpectedInvocations, "observed plugin invocations that were not expected")
}

// testServerAndClientConfig returns a server that listens and a config that can reference it
func testServerAndClientConfig(handler http.Handler) (*httptest.Server, *rest.Config) {
	srv := httptest.NewServer(handler)
	config := &rest.Config{
		Host: srv.URL,
	}
	return srv, config
}

func restMapperFromScheme(scheme *runtime.Scheme) meta.RESTMapper {
	rm := meta.NewDefaultRESTMapper(nil, meta.InterfacesForUnstructured)
	for gvk := range scheme.AllKnownTypes() {
		rm.Add(gvk, meta.RESTScopeNamespace)
	}
	return rm
}

// fakeAction records information about requests to aid in testing.
type fakeAction struct {
	method string
	path   string
	query  string
}

// String returns method=path to aid in testing
func (f *fakeAction) String() string {
	if strings.Contains(f.query, "watch=true") {
		return strings.Join([]string{f.method, f.path, "watch"}, "=")
	}
	if f.query != "" {
		return strings.Join([]string{f.method, f.path, f.query}, "=")
	}
	return strings.Join([]string{f.method, f.path}, "=")
}

type fakeResponse struct {
	statusCode int
	content    []byte
}

type path struct {
	method string
	path   string
	watch  bool
}

// fakeActionHandler holds a list of fakeActions received
type fakeActionHandler struct {
	// statusCode and content returned by this handler for different method + path.
	response map[path]fakeResponse

	lock    sync.Mutex
	actions []fakeAction
}

func (f *fakeActionHandler) getActions() []fakeAction {
	f.lock.Lock()
	defer f.lock.Unlock()
	result := make([]fakeAction, len(f.actions))
	copy(result, f.actions)
	return result
}

// ServeHTTP logs the action that occurred and always returns the associated status code
func (f *fakeActionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.actions = append(f.actions, fakeAction{method: request.Method, path: request.URL.Path, query: request.URL.RawQuery})
	key := path{method: request.Method, path: request.URL.Path, watch: strings.Contains(request.URL.RawQuery, "watch=true")}
	fakeResp, ok := f.response[key]
	if !ok {
		fakeResp = fakeResponse{
			statusCode: http.StatusInternalServerError,
		}
	} else if len(fakeResp.content) != 0 {
		response.Header().Set("Content-Type", "application/json")
	}
	response.WriteHeader(fakeResp.statusCode)
	response.Write(fakeResp.content)
}

func SleeperCrdWithStatus() *apiext_v1b1.CustomResourceDefinition {
	crd := sleeper.SleeperCrd()
	crd.Status = apiext_v1b1.CustomResourceDefinitionStatus{
		Conditions: []apiext_v1b1.CustomResourceDefinitionCondition{
			{Type: apiext_v1b1.Established, Status: apiext_v1b1.ConditionTrue},
			{Type: apiext_v1b1.NamesAccepted, Status: apiext_v1b1.ConditionTrue},
		},
	}
	return crd
}

func convertBundleResourcesToUnstrucutred(t *testing.T, bundle *smith_v1.Bundle, serviceCatalog bool) {
	// Convert all typed objects into unstructured ones
	for i, res := range bundle.Spec.Resources {
		if res.Spec.Object != nil {
			resUnstr, err := util.RuntimeToUnstructured(res.Spec.Object)
			require.NoError(t, err)
			bundle.Spec.Resources[i].Spec.Object = resUnstr
		}
	}
}

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

func (p *configMapWithDeps) Process(pluginSpec map[string]interface{}, context *plugin.Context) (*plugin.ProcessResult, error) {
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
		return nil, errors.New("plugin failed BOOM!")
	}

	return &plugin.ProcessResult{
		Object: &core_v1.ConfigMap{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
		},
	}, nil
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

func (p *simpleConfigMap) Process(pluginSpec map[string]interface{}, context *plugin.Context) (*plugin.ProcessResult, error) {
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
		return nil, errors.New("plugin failed BOOM!")
	}

	return &plugin.ProcessResult{
		Object: &core_v1.ConfigMap{
			TypeMeta: meta_v1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: core_v1.SchemeGroupVersion.String(),
			},
		},
	}, nil
}

func serviceInstance(ready, inProgress, error bool) *sc_v1b1.ServiceInstance {
	tr := true
	var status sc_v1b1.ServiceInstanceStatus
	if ready {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceInstanceCondition{
			Type:   sc_v1b1.ServiceInstanceConditionReady,
			Status: sc_v1b1.ConditionTrue,
		})
	}
	if inProgress {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceInstanceCondition{
			Type:    sc_v1b1.ServiceInstanceConditionReady,
			Status:  sc_v1b1.ConditionFalse,
			Reason:  "WorkingOnIt",
			Message: "Doing something",
		})
	}
	if error {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceInstanceCondition{
			Type:    sc_v1b1.ServiceInstanceConditionFailed,
			Status:  sc_v1b1.ConditionTrue,
			Reason:  "BlaBla",
			Message: "Oh no!",
		})
	}
	return &sc_v1b1.ServiceInstance{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceInstance",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      si1,
			Namespace: testNamespace,
			UID:       si1uid,
			Labels: map[string]string{
				smith.BundleNameLabel: bundle1,
			},
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion:         smith_v1.BundleResourceGroupVersion,
					Kind:               smith_v1.BundleResourceKind,
					Name:               bundle1,
					UID:                bundle1uid,
					Controller:         &tr,
					BlockOwnerDeletion: &tr,
				},
			},
		},
		Status: status,
		Spec:   serviceInstanceSpec,
	}
}

func serviceBinding(ready, inProgress, error bool) *sc_v1b1.ServiceBinding {
	tr := true
	var status sc_v1b1.ServiceBindingStatus
	if ready {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceBindingCondition{
			Type:   sc_v1b1.ServiceBindingConditionReady,
			Status: sc_v1b1.ConditionTrue,
		})
	}
	if inProgress {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceBindingCondition{
			Type:    sc_v1b1.ServiceBindingConditionReady,
			Status:  sc_v1b1.ConditionFalse,
			Reason:  "WorkingOnIt",
			Message: "Doing something",
		})
	}
	if error {
		status.Conditions = append(status.Conditions, sc_v1b1.ServiceBindingCondition{
			Type:    sc_v1b1.ServiceBindingConditionFailed,
			Status:  sc_v1b1.ConditionTrue,
			Reason:  "BlaBla",
			Message: "Oh no!",
		})
	}
	return &sc_v1b1.ServiceBinding{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ServiceBinding",
			APIVersion: sc_v1b1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      sb1,
			Namespace: testNamespace,
			UID:       sb1uid,
			Labels: map[string]string{
				smith.BundleNameLabel: bundle1,
			},
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion:         smith_v1.BundleResourceGroupVersion,
					Kind:               smith_v1.BundleResourceKind,
					Name:               bundle1,
					UID:                bundle1uid,
					Controller:         &tr,
					BlockOwnerDeletion: &tr,
				},
				{
					APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
					Kind:               "ServiceInstance",
					Name:               si1,
					UID:                si1uid,
					BlockOwnerDeletion: &tr,
				},
			},
		},
		Spec: sc_v1b1.ServiceBindingSpec{
			ServiceInstanceRef: sc_v1b1.LocalObjectReference{
				Name: si1,
			},
			SecretName: s1,
		},
		Status: status,
	}
}

func configMapNeedsUpdate() *core_v1.ConfigMap {
	tr := true
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      mapNeedsAnUpdate,
			Namespace: testNamespace,
			UID:       mapNeedsAnUpdateUid,
			// Labels missing - needs an update
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion:         smith_v1.BundleResourceGroupVersion,
					Kind:               smith_v1.BundleResourceKind,
					Name:               bundle1,
					UID:                bundle1uid,
					Controller:         &tr,
					BlockOwnerDeletion: &tr,
				},
			},
		},
		Data: map[string]string{
			"delete": "this key",
		},
	}
}

func configMapNeedsUpdateResponse(bundleName, bundleUid string) fakeResponse {
	return fakeResponse{
		statusCode: http.StatusOK,
		content: []byte(`{
							"apiVersion": "v1",
							"kind": "ConfigMap",
							"metadata": {
								"name": "` + mapNeedsAnUpdate + `",
								"namespace": "` + testNamespace + `",
								"uid": "` + mapNeedsAnUpdateUid + `",
								"labels": {
									"` + smith.BundleNameLabel + `": "` + bundleName + `"
								},
								"ownerReferences": [{
									"apiVersion": "` + smith_v1.BundleResourceGroupVersion + `",
									"kind": "` + smith_v1.BundleResourceKind + `",
									"name": "` + bundleName + `",
									"uid": "` + bundleUid + `",
									"controller": true,
									"blockOwnerDeletion": true
								}] }
							}`),
	}
}

func configMapNeedsDelete() *core_v1.ConfigMap {
	tr := true
	return &core_v1.ConfigMap{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: core_v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      mapNeedsDelete,
			Namespace: testNamespace,
			UID:       mapNeedsDeleteUid,
			Labels: map[string]string{
				smith.BundleNameLabel: bundle1,
			},
			OwnerReferences: []meta_v1.OwnerReference{
				{
					APIVersion:         smith_v1.BundleResourceGroupVersion,
					Kind:               smith_v1.BundleResourceKind,
					Name:               bundle1,
					UID:                bundle1uid,
					Controller:         &tr,
					BlockOwnerDeletion: &tr,
				},
			},
		},
	}
}
