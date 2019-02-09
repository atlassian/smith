package bundlec_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/ctrl/process"
	"github.com/atlassian/smith/cmd/smith/app"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	smithFake "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/fake"
	"github.com/atlassian/smith/pkg/client/smart"
	"github.com/atlassian/smith/pkg/controller/bundlec"
	"github.com/atlassian/smith/pkg/crd"
	"github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/util"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	scFake "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiExtFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	"k8s.io/apimachinery/pkg/api/meta"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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
	logger             *zap.Logger
	mainClientObjects  []runtime.Object
	mainReactors       []reaction
	smithClientObjects []runtime.Object
	smithReactors      []reaction
	// Bundle CRD is added automatically
	apiExtClientObjects []runtime.Object
	apiExtReactors      []reaction
	scClientObjects     []runtime.Object
	scReactors          []reaction
	bundle              *smith_v1.Bundle
	namespace           string
	appName             string

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
	testAppName   = "testapp"
	testNamespace = "test-namespace"

	resSb1           = "resSb1"
	sb1              = "sb1"
	sb1uid types.UID = "sb1-uid"

	resSi1           = "resSi1"
	si1              = "si1"
	si1uid types.UID = "si1-uid"
	resSi2           = "resSi2"
	si2              = "si2"

	s1              = "s1"
	s1uid types.UID = "s1-uid"

	m1 = "m1"

	resP1 = "resP1"

	resSleeper1           = "resSleeper1"
	sleeper1              = "sleeper1"
	sleeper1uid types.UID = "sleeper1-uid"

	bundle1              = "bundle1"
	bundle1uid types.UID = "bundle1-uid"

	resMapNeedsAnUpdate           = "res-map-needs-update"
	mapNeedsAnUpdate              = "map-needs-update"
	mapNeedsAnUpdateUid types.UID = "map-needs-update-uid"

	mapNeedsDelete              = "map-not-in-the-bundle-anymore-needs-delete"
	mapNeedsDeleteUid types.UID = "map-needs-delete-uid"

	mapMarkedForDeletion              = "map-not-in-the-bundle-anymore-marked-for-deletetion"
	mapMarkedForDeletionUid types.UID = "map-deleted-uid"

	pluginMockConfigMap     smith_v1.PluginName = "mockConfigMap"
	pluginSimpleConfigMap   smith_v1.PluginName = "simpleConfigMap"
	pluginConfigMapWithDeps smith_v1.PluginName = "configMapWithDeps"
	pluginFailing           smith_v1.PluginName = "pluginFailing"

	serviceClassNameAndID    = "uid-1"
	serviceClassExternalName = "database"
	servicePlanNameAndID     = "uid-2"
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
		tc.bundle.TypeMeta = meta_v1.TypeMeta{
			Kind:       smith_v1.BundleResourceKind,
			APIVersion: smith_v1.BundleResourceGroupVersion,
		}
		tc.bundle.ObjectMeta.SelfLink = fmt.Sprintf("/apis/%s/namespaces/%s/%s/%s", tc.bundle.TypeMeta.APIVersion, tc.bundle.Namespace, smith_v1.BundleResourcePlural, tc.bundle.Name)
		convertBundleResourcesToUnstrucutred(t, tc.bundle, tc.enableServiceCatalog)
		tc.smithClientObjects = append(tc.smithClientObjects, tc.bundle)
	}
	smithClient := smithFake.NewSimpleClientset(tc.smithClientObjects...)
	tc.smithFake = &smithClient.Fake
	for _, reactor := range tc.smithReactors {
		smithClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	scheme, err := app.FullScheme(tc.enableServiceCatalog)
	require.NoError(t, err)
	apiExtObjects := append(tc.apiExtClientObjects, crd.BundleCrd())
	for _, object := range apiExtObjects {
		// Pretend that the object has been defaulted by the server. We need this to set the version field
		scheme.Default(object)
	}
	apiExtClient := apiExtFake.NewSimpleClientset(apiExtObjects...)
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
						Name: serviceClassNameAndID,
					},
					Spec: sc_v1b1.ClusterServiceClassSpec{
						CommonServiceClassSpec: sc_v1b1.CommonServiceClassSpec{
							ExternalName: serviceClassExternalName,
							ExternalID:   serviceClassNameAndID,
						},
					},
				},
				&sc_v1b1.ClusterServicePlan{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ClusterServicePlan",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name: servicePlanNameAndID,
					},
					Spec: sc_v1b1.ClusterServicePlanSpec{
						ClusterServiceClassRef: sc_v1b1.ClusterObjectReference{
							Name: serviceClassNameAndID,
						},
						CommonServicePlanSpec: sc_v1b1.CommonServicePlanSpec{
							ExternalName: servicePlanExternalName,
							ExternalID:   servicePlanNameAndID,
							InstanceCreateParameterSchema: &runtime.RawExtension{Raw: []byte(`
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

	for _, object := range tc.apiExtClientObjects {
		crd := object.(*apiext_v1b1.CustomResourceDefinition)
		scheme.AddKnownTypeWithName(schema.GroupVersionKind{
			Group:   crd.Spec.Group,
			Version: crd.Spec.Versions[0].Name,
			Kind:    crd.Spec.Names.Kind,
			// obj: unstructured.Unstructured
			// is here _only_ to keep rest scheme happy, we do not currently use scheme to deserialize
		}, &unstructured.Unstructured{})
	}
	restMapper := restMapperFromScheme(scheme)

	tc.logger = zaptest.NewLogger(t)
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
	prometheusRegistry := prometheus.NewPedanticRegistry()
	config := &ctrl.Config{
		Logger:     tc.logger,
		Namespace:  tc.namespace,
		RestConfig: clientConfig,
		MainClient: mainClient,
		AppName:    tc.appName,
		Registry:   prometheusRegistry,
	}
	dynamicClient, err := dynamic.NewForConfig(clientConfig)
	require.NoError(t, err)
	bundleConstr := &app.BundleControllerConstructor{
		Plugins:               plugins,
		ServiceCatalogSupport: tc.enableServiceCatalog,
		SmithClient:           smithClient,
		SCClient:              scClient,
		APIExtClient:          apiExtClient,
		SmartClient: &smart.DynamicClient{
			DynamicClient: dynamicClient,
			RESTMapper:    restMapper,
		},
	}
	generic, err := process.NewGeneric(config,
		workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "multiqueue"),
		2, bundleConstr)
	require.NoError(t, err)
	cntrlr := generic.Controllers[smith_v1.BundleGVK].Cntrlr.(*bundlec.Controller)

	// Start all informers then wait on them
	for _, inf := range generic.Informers {
		stage.StartWithChannel(inf.Run) // Must be after AddInformer()
	}
	for _, inf := range generic.Informers {
		require.True(t, cache.WaitForCacheSync(ctx.Done(), inf.HasSynced))
	}

	defer tc.verifyActions(t)
	defer tc.verifyPluginStatuses(t)
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
	require.NoError(t, err)
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

func (tc *testCase) findBundleUpdate(t *testing.T, onlyOne bool) *smith_v1.Bundle {
	var bundle *smith_v1.Bundle
	for _, action := range tc.smithFake.Actions() {
		update, ok := action.(kube_testing.UpdateAction)
		if !ok {
			t.Logf("%T is not an update action", action)
			continue
		}
		if update.GetNamespace() != tc.namespace {
			t.Logf("%q is not the test namespace %q", update.GetNamespace(), tc.namespace)
			continue
		}
		updateBundle, ok := update.GetObject().(*smith_v1.Bundle)
		if !ok {
			t.Logf("%T is not a *Bundle", update.GetObject())
			continue
		}
		if onlyOne {
			assert.Nil(t, bundle, "Duplicate Bundle update found: %v", update)
		}
		bundle = updateBundle
	}
	return bundle
}

func (tc *testCase) verifyPluginStatuses(t *testing.T) {
	bundle := tc.findBundleUpdate(t, true)
	if bundle == nil {
		t.Log("No Bundle updates found, not checking plugin statuses")
		return
	}
	pluginNames := map[smith_v1.PluginName]struct{}{}
	for _, res := range tc.bundle.Spec.Resources {
		if res.Spec.Plugin == nil {
			continue
		}
		pluginNames[res.Spec.Plugin.Name] = struct{}{}
	}
	assert.Len(t, bundle.Status.PluginStatuses, len(pluginNames))
nextPlugin:
	for _, pluginStatus := range bundle.Status.PluginStatuses {
		for _, constructedPlugin := range tc.pluginsConstructed {
			describe := constructedPlugin.Describe()
			if pluginStatus.Name != describe.Name {
				continue
			}
			assert.Equal(t, smith_v1.PluginStatusOk, pluginStatus.Status)
			assert.Equal(t, describe.GVK, schema.GroupVersionKind{
				Group:   pluginStatus.Group,
				Version: pluginStatus.Version,
				Kind:    pluginStatus.Kind,
			})
			continue nextPlugin
		}

		if pluginStatus.Status != smith_v1.PluginStatusNoSuchPlugin {
			// We should have found it?
			t.Errorf("plugin status for %s that has not been constructed", pluginStatus.Name)
		}
	}
}

func (tc *testCase) assertObjectsToBeDeleted(t *testing.T, objs ...runtime.Object) {
	// Must be in same order
	expected := make([]smith_v1.ObjectToDelete, 0, len(objs))
	for _, obj := range objs {
		gvk := obj.GetObjectKind().GroupVersionKind()
		expected = append(expected, smith_v1.ObjectToDelete{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
			Name:    obj.(meta_v1.Object).GetName(),
		})
	}
	assert.Equal(t, expected, tc.bundle.Status.ObjectsToDelete)
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
	rm := meta.NewDefaultRESTMapper(nil)
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
