package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atlassian/smith"
	"github.com/atlassian/smith/examples/sleeper"
	"github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/cleanup"
	clean_types "github.com/atlassian/smith/pkg/cleanup/types"
	"github.com/atlassian/smith/pkg/client"
	smithFake "github.com/atlassian/smith/pkg/client/clientset_generated/clientset/fake"
	"github.com/atlassian/smith/pkg/client/smart"
	smith_plugin "github.com/atlassian/smith/pkg/plugin"
	"github.com/atlassian/smith/pkg/readychecker"
	ready_types "github.com/atlassian/smith/pkg/readychecker/types"
	"github.com/atlassian/smith/pkg/resources/apitypes"
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/store"
	"github.com/atlassian/smith/pkg/util"

	"github.com/ash2k/stager"
	sc_v1b1 "github.com/kubernetes-incubator/service-catalog/pkg/apis/servicecatalog/v1beta1"
	scClientset "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset"
	scFake "github.com/kubernetes-incubator/service-catalog/pkg/client/clientset_generated/clientset/fake"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	apiext_v1b1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crdFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	crdInformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
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
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type reaction struct {
	verb     string
	resource string
	reactor  func(*testing.T) core.ReactionFunc
}

type testCase struct {
	mainClientObjects  []runtime.Object
	mainReactors       []reaction
	smithClientObjects []runtime.Object
	smithReactors      []reaction
	crdClientObjects   []runtime.Object
	crdReactors        []reaction
	scClientObjects    []runtime.Object
	scReactors         []reaction
	bundle             *smith_v1.Bundle
	namespace          string

	expectedActions      sets.String
	enableServiceCatalog bool
	testHandler          fakeActionHandler
	test                 func(*testing.T, context.Context, *BundleController, *testCase)
	plugins              map[string]func(*testing.T) smith_plugin.Func
}

const (
	testNamespace = "test-namespace"
)

func TestController(t *testing.T) {
	t.Parallel()
	tr := true
	testcases := map[string]*testCase{
		"deletes owned object that is not in bundle": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.ConfigMap{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "map1",
						Namespace: testNamespace,
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
			},
			expectedActions: sets.NewString("DELETE=/api/v1/namespaces/" + testNamespace + "/configmaps/map1"),
			namespace:       meta_v1.NamespaceAll,
		},
		"can list crds in another namespace": &testCase{
			crdClientObjects: []runtime.Object{
				SleeperCrdWithStatus(),
			},
			expectedActions: sets.NewString(
				"GET=/apis/"+v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+v1.SleeperResourcePlural+
					"=resourceVersion=0",
				"GET=/apis/"+v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+v1.SleeperResourcePlural+
					"=watch",
			),
			namespace: testNamespace,
			test: func(t *testing.T, ctx context.Context, bundleController *BundleController, testcase *testCase) {
				subContext, _ := context.WithTimeout(ctx, 1000*time.Millisecond)
				bundleController.Run(subContext)
			},
			testHandler: fakeActionHandler{
				response: map[path]fakeResponse{
					{
						method: "GET",
						watch:  true,
						path:   "/apis/" + v1.SleeperResourceGroupVersion + "/namespaces/" + testNamespace + "/" + v1.SleeperResourcePlural,
					}: {
						statusCode: http.StatusOK,
						content:    []byte(`{"type": "ADDED", "object": { "kind": "Unknown" } }`),
					},
				},
			},
		},
		"plugin spec is processed": &testCase{
			mainClientObjects: []runtime.Object{
				&core_v1.Secret{
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "s1",
						Namespace: testNamespace,
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
								Kind:               "ServiceBinding",
								Name:               "sb1",
								UID:                types.UID("sb1-uid"),
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Data: map[string][]byte{
						"data": []byte("bla"),
					},
					Type: core_v1.SecretTypeOpaque,
				},
			},
			scClientObjects: []runtime.Object{
				&sc_v1b1.ServiceInstance{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceInstance",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "si1",
						Namespace: testNamespace,
						UID:       types.UID("si1-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Status: sc_v1b1.ServiceInstanceStatus{
						Conditions: []sc_v1b1.ServiceInstanceCondition{
							{
								Type:   sc_v1b1.ServiceInstanceConditionReady,
								Status: sc_v1b1.ConditionTrue,
							},
						},
					},
				},
				&sc_v1b1.ServiceBinding{
					TypeMeta: meta_v1.TypeMeta{
						Kind:       "ServiceBinding",
						APIVersion: sc_v1b1.SchemeGroupVersion.String(),
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "sb1",
						Namespace: testNamespace,
						UID:       types.UID("sb1-uid"),
						Labels: map[string]string{
							smith.BundleNameLabel: "bundle1",
						},
						OwnerReferences: []meta_v1.OwnerReference{
							{
								APIVersion:         smith_v1.BundleResourceGroupVersion,
								Kind:               smith_v1.BundleResourceKind,
								Name:               "bundle1",
								UID:                "uid123",
								Controller:         &tr,
								BlockOwnerDeletion: &tr,
							},
							{
								APIVersion:         sc_v1b1.SchemeGroupVersion.String(),
								Kind:               "ServiceInstance",
								Name:               "si1",
								UID:                "si1-uid",
								BlockOwnerDeletion: &tr,
							},
						},
					},
					Spec: sc_v1b1.ServiceBindingSpec{
						ServiceInstanceRef: sc_v1b1.LocalObjectReference{
							Name: "si1",
						},
						SecretName: "s1",
					},
					Status: sc_v1b1.ServiceBindingStatus{
						Conditions: []sc_v1b1.ServiceBindingCondition{
							{
								Type:   sc_v1b1.ServiceBindingConditionReady,
								Status: sc_v1b1.ConditionTrue,
							},
						},
					},
				},
			},
			bundle: &smith_v1.Bundle{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "bundle1",
					Namespace: testNamespace,
					UID:       "uid123",
				},
				Spec: smith_v1.BundleSpec{
					Resources: []smith_v1.Resource{
						{
							Name: "si1",
							Spec: &sc_v1b1.ServiceInstance{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ServiceInstance",
									APIVersion: sc_v1b1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: "si1",
								},
							},
						},
						{
							Name: "sb1",
							DependsOn: []smith_v1.ResourceName{
								"si1",
							},
							Spec: &sc_v1b1.ServiceBinding{
								TypeMeta: meta_v1.TypeMeta{
									Kind:       "ServiceBinding",
									APIVersion: sc_v1b1.SchemeGroupVersion.String(),
								},
								ObjectMeta: meta_v1.ObjectMeta{
									Name: "sb1",
								},
								Spec: sc_v1b1.ServiceBindingSpec{
									ServiceInstanceRef: sc_v1b1.LocalObjectReference{
										Name: "si1",
									},
									SecretName: "s1",
								},
							},
						},
						{
							Name: "p1",
							DependsOn: []smith_v1.ResourceName{
								"sb1",
							},
							Type:       smith_v1.Plugin,
							PluginName: "testPlugin",
							PluginSpec: &smith_v1.PluginSpec{
								Kind:       "ConfigMap",
								ApiVersion: core_v1.SchemeGroupVersion.String(),
								Name:       "m1",
							},
						},
					},
				},
			},
			namespace:            testNamespace,
			expectedActions:      sets.NewString("POST=/api/v1/namespaces/" + testNamespace + "/configmaps"),
			enableServiceCatalog: true,
			testHandler: fakeActionHandler{
				response: map[path]fakeResponse{
					{
						method: "POST",
						path:   "/api/v1/namespaces/" + testNamespace + "/configmaps",
					}: {
						statusCode: http.StatusCreated,
						content:    []byte(`{ "apiVersion": "v1", "kind": "ConfigMap", "metadata": { "name": "m1", "namespace": "` + testNamespace + `" } }`),
					},
				},
			},
			plugins: map[string]func(*testing.T) smith_plugin.Func{
				"testPlugin": testPlugin,
			},
		},
	}
	for name, tc := range testcases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			defer tc.verifyActions(t)
			tc.run(t)
		})
	}
}

func (tc *testCase) run(t *testing.T) {
	mainClient := mainFake.NewSimpleClientset(tc.mainClientObjects...)
	for _, reactor := range tc.mainReactors {
		mainClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	if tc.bundle != nil {
		for i, res := range tc.bundle.Spec.Resources {
			if res.Type == smith_v1.Normal {
				resUnstr, err := util.RuntimeToUnstructured(res.Spec)
				require.NoError(t, err)
				tc.bundle.Spec.Resources[i].Spec = resUnstr
			}
		}
		tc.smithClientObjects = append(tc.smithClientObjects, tc.bundle)
	}
	bundleClient := smithFake.NewSimpleClientset(tc.smithClientObjects...)
	for _, reactor := range tc.smithReactors {
		bundleClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	crdClient := crdFake.NewSimpleClientset(tc.crdClientObjects...)
	for _, reactor := range tc.crdReactors {
		crdClient.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
	}
	var scClientFake *scFake.Clientset
	var scClient scClientset.Interface
	if tc.enableServiceCatalog {
		scClientFake = scFake.NewSimpleClientset(tc.scClientObjects...)
		scClient = scClientFake
		for _, reactor := range tc.scReactors {
			scClientFake.AddReactor(reactor.verb, reactor.resource, reactor.reactor(t))
		}
	}

	informerFactory := crdInformers.NewSharedInformerFactory(crdClient, 0)
	crdInf := informerFactory.Apiextensions().V1beta1().CustomResourceDefinitions().Informer()
	bundleInf := client.BundleInformer(bundleClient.SmithV1(), meta_v1.NamespaceAll, 0)
	scheme, err := apitypes.FullScheme(tc.enableServiceCatalog)

	for _, object := range tc.crdClientObjects {
		crd := object.(*apiext_v1b1.CustomResourceDefinition)
		scheme.AddKnownTypeWithName(schema.GroupVersionKind{
			Group:   crd.Spec.Group,
			Version: crd.Spec.Version,
			Kind:    crd.Spec.Names.Kind,
			// obj: unstructured.Unstructured
			// is here _only_ to keep rest scheme happy, we do not currently use scheme to deserialize
		}, &unstructured.Unstructured{})
	}

	multiStore := store.NewMulti()

	bs, err := store.NewBundle(bundleInf, multiStore)
	require.NoError(t, err)
	resourceInfs := apitypes.ResourceInformers(mainClient, scClient, meta_v1.NamespaceAll, 0, true)

	crdStore, err := store.NewCrd(crdInf)
	require.NoError(t, err)

	// Ready Checker
	readyTypes := []map[schema.GroupKind]readychecker.IsObjectReady{ready_types.MainKnownTypes}
	if tc.enableServiceCatalog {
		readyTypes = append(readyTypes, ready_types.ServiceCatalogKnownTypes)
	}
	rc := readychecker.New(crdStore, readyTypes...)

	// Object cleanup
	cleanupTypes := []map[schema.GroupKind]cleanup.SpecCleanup{clean_types.MainKnownTypes}
	if tc.enableServiceCatalog {
		cleanupTypes = append(cleanupTypes, clean_types.ServiceCatalogKnownTypes)
	}
	oc := cleanup.New(cleanupTypes...)

	// Spec check
	specCheck := &speccheck.SpecCheck{
		Scheme:  scheme,
		Cleaner: oc,
	}

	// Look at k8s.io/kubernetes/pkg/controller/garbagecollector/garbagecollector_test.go for inspiration
	srv, clientConfig := testServerAndClientConfig(&tc.testHandler)
	defer srv.Close()

	restMapper := restMapperFromScheme(scheme)

	sc := &smart.DynamicClient{
		CoreDynamic: dynamic.NewClientPool(clientConfig, restMapper, dynamic.LegacyAPIPathResolverFunc),
		Mapper:      restMapper,
	}
	if tc.enableServiceCatalog {
		sc.ScDynamic = sc.CoreDynamic
		sc.ScMapper = restMapper
	}

	stgr := stager.New()
	defer stgr.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	plugins := make(map[string]smith_plugin.Func, len(tc.plugins))
	for name, factory := range tc.plugins {
		plugins[name] = factory(t)
	}

	c := New(
		bundleInf,
		crdInf,
		bundleClient.SmithV1(),
		bs,
		sc,
		rc,
		multiStore,
		specCheck,
		workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "bundle"),
		2,
		0,
		resourceInfs,
		tc.namespace,
		plugins,
		scheme)

	crdGVK := apiext_v1b1.SchemeGroupVersion.WithKind("CustomResourceDefinition")
	resourceInfs[crdGVK] = crdInf
	resourceInfs[smith_v1.BundleGVK] = bundleInf
	infs := make([]cache.InformerSynced, 0, len(resourceInfs))
	stage := stgr.NextStage()
	for gvk, inf := range resourceInfs {
		multiStore.AddInformer(gvk, inf)
		stage.StartWithChannel(inf.Run)
		infs = append(infs, inf.HasSynced)
	}
	require.True(t, cache.WaitForCacheSync(ctx.Done(), infs...))

	if tc.test == nil {
		// Default test
		require.NotNil(t, tc.bundle)
		key, err := cache.MetaNamespaceKeyFunc(tc.bundle)
		require.NoError(t, err)
		_, err = c.processKey(key)
		require.NoError(t, err)
	} else {
		tc.test(t, ctx, c, tc)
	}
}

func (tc *testCase) verifyActions(t *testing.T) {
	actualActions := sets.NewString()
	for _, actualAction := range tc.testHandler.actions {
		actualActions.Insert(actualAction.String())
	}
	missingActions := tc.expectedActions.Difference(actualActions)
	unexpectedActions := actualActions.Difference(tc.expectedActions)
	assert.Empty(t, missingActions, "expected but was not observed")
	assert.Empty(t, unexpectedActions, "observed but was not expected")
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

// ServeHTTP logs the action that occurred and always returns the associated status code
func (f *fakeActionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.actions = append(f.actions, fakeAction{method: request.Method, path: request.URL.Path, query: request.URL.RawQuery})
	key := path{method: request.Method, path: request.URL.Path, watch: strings.Contains(request.URL.RawQuery, "watch=true")}
	fakeResp, ok := f.response[key]
	if !ok {
		fakeResp = fakeResponse{
			statusCode: http.StatusOK,
			content:    []byte(`{"kind": "List", "items": []}`),
		}
	}
	response.Header().Set("Content-Type", "application/json")
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

func testPlugin(t *testing.T) smith_plugin.Func {
	return func(resource smith_v1.Resource, dependencies map[smith_v1.ResourceName]smith_plugin.Dependency) (smith_plugin.ProcessResult, error) {
		failed := t.Failed()
		assert.Equal(t, "testPlugin", resource.PluginName)
		assert.Equal(t, []smith_v1.ResourceName{"sb1"}, resource.DependsOn)
		assert.Len(t, dependencies, 1)
		bindingDep, ok := dependencies["sb1"]
		if assert.True(t, ok) {
			// Actual
			if assert.IsType(t, &sc_v1b1.ServiceBinding{}, bindingDep.Actual) {
				b := bindingDep.Actual.(*sc_v1b1.ServiceBinding)
				assert.Equal(t, "sb1", b.Name)
				assert.Equal(t, testNamespace, b.Namespace)
				assert.Equal(t, sc_v1b1.SchemeGroupVersion.WithKind("ServiceBinding"), b.GroupVersionKind())
			}
			// Outputs
			if assert.Len(t, bindingDep.Outputs, 1) {
				secret := bindingDep.Outputs[0]
				if assert.IsType(t, &core_v1.Secret{}, secret) {
					s := secret.(*core_v1.Secret)
					assert.Equal(t, "s1", s.Name)
					assert.Equal(t, testNamespace, s.Namespace)
					assert.Equal(t, core_v1.SchemeGroupVersion.WithKind("Secret"), s.GroupVersionKind())
				}
			}
			// Aux
			if assert.Len(t, bindingDep.Auxiliary, 1) {
				svcInst := bindingDep.Auxiliary[0]
				if assert.IsType(t, &sc_v1b1.ServiceInstance{}, svcInst) {
					inst := svcInst.(*sc_v1b1.ServiceInstance)
					assert.Equal(t, "si1", inst.Name)
					assert.Equal(t, testNamespace, inst.Namespace)
					assert.Equal(t, sc_v1b1.SchemeGroupVersion.WithKind("ServiceInstance"), inst.GroupVersionKind())
				}
			}
		}

		if !failed && t.Failed() { // one of the assertions failed and it was the first failure in the test
			return smith_plugin.ProcessResult{}, errors.New("plugin failed BOOM!")
		}

		return smith_plugin.ProcessResult{
			Object: &core_v1.ConfigMap{
				TypeMeta: meta_v1.TypeMeta{
					Kind:       "ConfigMap",
					APIVersion: core_v1.SchemeGroupVersion.String(),
				},
				ObjectMeta: meta_v1.ObjectMeta{
					Name: resource.PluginSpec.Name,
				},
			},
		}, nil
	}
}
