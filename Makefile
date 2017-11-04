METALINTER_CONCURRENCY ?= 4
ALL_GO_FILES=$$(find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./build/*" -not -path './pkg/client/clientset_generated/*' -not -name 'zz_generated.*')
GOVERSION := 1.9.2
GP := /gopath
GOPATH ?= "$$HOME/go"

setup: setup-ci
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install

setup-ci:
	dep ensure
	# workaround https://github.com/kubernetes/kubernetes/issues/50975
	cp fixed_BUILD_for_sets.bazel vendor/k8s.io/apimachinery/pkg/util/sets/BUILD
	go build -o build/bin/buildozer vendor/github.com/bazelbuild/buildtools/buildozer/*.go
	rm -rf vendor/github.com/bazelbuild
	bazel run //:gazelle-fix

update-bazel:
	-build/bin/buildozer 'set race "on"' \
		//cmd/...:%go_test \
		//examples/...:%go_test \
		//it/...:%go_test \
		//pkg/...:%go_test
	bazel run //:gazelle

build: fmt update-bazel build-ci

build-race: fmt update-bazel
	bazel build --features=race //cmd/smith

build-ci:
	bazel build //cmd/smith

fmt:
	goimports -w=true -d $(ALL_GO_FILES)

generate: generate-client generate-deepcopy

generate-client:
	bazel build //vendor/k8s.io/code-generator/cmd/client-gen
	# Generate the versioned clientset (pkg/client/clientset_generated/clientset)
	bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/client-gen $(VERIFY_CODE) \
	--input-base "github.com/atlassian/smith/pkg/apis/" \
	--input "smith/v1" \
	--clientset-path "github.com/atlassian/smith/pkg/client/clientset_generated/" \
	--clientset-name "clientset" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

generate-deepcopy:
	bazel build //vendor/k8s.io/code-generator/cmd/deepcopy-gen
	# Generate deep copies
	bazel-bin/vendor/k8s.io/code-generator/cmd/deepcopy-gen/deepcopy-gen $(VERIFY_CODE) \
	--v 1 --logtostderr \
	--go-header-file "build/code-generator/boilerplate.go.txt" \
	--input-dirs "github.com/atlassian/smith/pkg/apis/smith/v1,github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1" \
	--bounding-dirs "github.com/atlassian/smith/pkg/apis/smith/v1,github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1" \
	--output-file-base zz_generated.deepcopy

minikube-test: fmt update-bazel
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
		--test_env=KUBERNETES_SERVICE_PORT=8443 \
		--test_env=KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
		--test_env=KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
		--test_env=KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
		//it:go_default_test

minikube-test-sc: fmt update-bazel
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
		--test_env=KUBERNETES_SERVICE_PORT=8443 \
		--test_env=KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
		--test_env=KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
		--test_env=KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
		--test_env=SERVICE_CATALOG_URL="http://$$(minikube ip):30080" \
		//it/sc:go_default_test

minikube-run: fmt update-bazel build
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel-bin/cmd/smith/smith -disable-service-catalog

minikube-run-sc: fmt update-bazel build
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel-bin/cmd/smith/smith  \
	-service-catalog-url="https://$$(minikube ip):30443" \
	-service-catalog-insecure

minikube-sleeper-run: fmt update-bazel
	bazel build --features=race //examples/sleeper/main
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel-bin/examples/sleeper/main/main

test: fmt update-bazel test-ci

verify:
	VERIFY_CODE=--verify-only make generate
	# TODO verify BUILD.bazel files are up to date

test-ci:
	# TODO: why does it build binaries and docker in cmd?
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- //... -//cmd/... -//vendor/...

check:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor \
		--linter='errcheck:errcheck:-ignore=net:Close' --cyclo-over=20 \
		--disable=interfacer --disable=golint --dupl-threshold=200

check-all:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor --cyclo-over=20 \
		--dupl-threshold=65

docker:
	bazel build --cpu=k8 //cmd/smith:container

# Export docker image into local Docker
docker-export:
	bazel run --cpu=k8 //cmd/smith:container

release: update-bazel
	bazel run --cpu=k8 //cmd/smith:push-docker

.PHONY: build
