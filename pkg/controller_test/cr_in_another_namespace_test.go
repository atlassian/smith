package controller_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	sleeper_v1 "github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1"
	"github.com/atlassian/smith/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Should be able to list CRs in another namespace
func TestCrInAnotherNamespace(t *testing.T) {
	t.Parallel()
	tc := testCase{
		crdClientObjects: []runtime.Object{
			SleeperCrdWithStatus(),
		},
		expectedActions: sets.NewString(
			"GET=/apis/"+sleeper_v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+sleeper_v1.SleeperResourcePlural+
				"=limit=500&resourceVersion=0",
			"GET=/apis/"+sleeper_v1.SleeperResourceGroupVersion+"/namespaces/"+testNamespace+"/"+sleeper_v1.SleeperResourcePlural+
				"=watch",
		),
		namespace: testNamespace,
		test: func(t *testing.T, ctx context.Context, bundleController *controller.BundleController, testcase *testCase) {
			bundleController.Run(ctx)
		},
		testHandler: fakeActionHandler{
			response: map[path]fakeResponse{
				{
					method: "GET",
					path:   "/apis/" + sleeper_v1.SleeperResourceGroupVersion + "/namespaces/" + testNamespace + "/" + sleeper_v1.SleeperResourcePlural,
				}: {
					statusCode: http.StatusOK,
					content:    []byte(`{"kind": "List", "items": []}`),
				},
				{
					method: "GET",
					watch:  true,
					path:   "/apis/" + sleeper_v1.SleeperResourceGroupVersion + "/namespaces/" + testNamespace + "/" + sleeper_v1.SleeperResourcePlural,
				}: {
					statusCode: http.StatusOK,
				},
			},
		},
		testTimeout: 500 * time.Millisecond,
	}
	tc.run(t)
}
