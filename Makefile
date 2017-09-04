METALINTER_CONCURRENCY ?= 4
ALL_GO_FILES=$$(find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./build/*" -not -path './pkg/client/clientset_generated/*' -not -name 'zz_generated.*')

setup: setup-ci update-bazel
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install

setup-ci:
	dep ensure
	# workaround https://github.com/kubernetes/kubernetes/issues/50975
	cp fixed_BUILD_for_sets.bazel vendor/k8s.io/apimachinery/pkg/util/sets/BUILD

update-bazel:
	bazel $(BAZEL_CI_OPTS) run //:gazelle

build: fmt update-bazel build-ci

build-race: fmt update-bazel
	bazel $(BAZEL_CI_OPTS) build //cmd/smith:smith-race

build-ci:
	bazel $(BAZEL_CI_OPTS) build //cmd/smith

fmt:
	gofmt -w=true -s $(ALL_GO_FILES)
	goimports -w=true -d $(ALL_GO_FILES)

generate: generate-client generate-deepcopy
	# Make sure you have k8s.io/kubernetes cloned into build/go/src/k8s.io/kubernetes
	# at revision ebb8d6e0fadfc95f3d64ccecc36c8ed2ac9224ef
	# TODO automate this. We'll use k8s.io/kube-gen instead once we are on 1.8 and that repo is published

generate-client:
	GOPATH=$(PWD)/build/go go build -i -o build/bin/client-gen k8s.io/kubernetes/cmd/libs/go2idl/client-gen
	# Generate the versioned clientset (pkg/client/clientset_generated/clientset)
	build/bin/client-gen \
	--input-base "github.com/atlassian/smith/pkg/apis/" \
	--input "smith/v1" \
	--clientset-path "github.com/atlassian/smith/pkg/client/clientset_generated/" \
	--clientset-name "clientset" \
	--go-header-file "build/boilerplate.go.txt"

generate-deepcopy:
	GOPATH=$(PWD)/build/go go build -i -o build/bin/deepcopy-gen k8s.io/kubernetes/cmd/libs/go2idl/deepcopy-gen
	# Generate deep copies
	build/bin/deepcopy-gen \
	--v 1 --logtostderr \
	--go-header-file "build/boilerplate.go.txt" \
	--input-dirs "github.com/atlassian/smith/pkg/apis/smith/v1" \
	--bounding-dirs "github.com/atlassian/smith" \
	--output-file-base zz_generated.deepcopy

minikube-test: fmt update-bazel
	bazel $(BAZEL_CI_OPTS) test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
		--test_env=KUBERNETES_SERVICE_PORT=8443 \
		--test_env=KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
		--test_env=KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
		--test_env=KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
		//it:go_default_test

minikube-test-sc: fmt update-bazel
	bazel $(BAZEL_CI_OPTS) test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
		--test_env=KUBERNETES_SERVICE_PORT=8443 \
		--test_env=KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
		--test_env=KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
		--test_env=KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
		--test_env=SERVICE_CATALOG_URL="http://$$(minikube ip):30080" \
		//it/sc:go_default_test

minikube-run: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel run //cmd/smith:smith-race

minikube-run-sc: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel run //cmd/smith:smith-race -- -service-catalog-url="https://$$(minikube ip):30443"

minikube-sleeper-run: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel run //examples/sleeper/main:main-race

test: fmt update-bazel test-ci

verify:
	# TODO verify BUILD.bazel files are up to date

test-ci:
	# TODO: why does it build binaries and docker in cmd?
	bazel $(BAZEL_CI_OPTS) test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- ... -cmd/... -vendor/...

check:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor \
		--linter='errcheck:errcheck:-ignore=net:Close' --cyclo-over=20 \
		--disable=interfacer --disable=golint --dupl-threshold=200

check-all:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor --cyclo-over=20 \
		--dupl-threshold=65

docker:
	bazel $(BAZEL_CI_OPTS) build --cpu=k8 //cmd/smith:docker

# Export docker image into local Docker
docker-export:
	bazel run --cpu=k8 //cmd/smith:docker

release:
	bazel $(BAZEL_CI_OPTS) run --cpu=k8 //cmd/smith:push-docker

.PHONY: build
