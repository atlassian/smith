METALINTER_CONCURRENCY ?= 4
ALL_GO_FILES=$$(find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./build/*" -not -path './pkg/client/clientset_generated/*' -not -name 'zz_generated.*')
OS = $$(uname -s | tr A-Z a-z)
BINARY_PREFIX_DIRECTORY=$(OS)_amd64_stripped
BINARY_PURE_PREFIX_DIRECTORY=$(OS)_amd64_pure_stripped
#BINARY_RACE_PREFIX_DIRECTORY=$(OS)_amd64_race_stripped

.PHONY: setup
setup: setup-ci
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install

.PHONY: setup-ci
setup-ci:
	dep ensure
	# workaround https://github.com/kubernetes/kubernetes/issues/50975
	cp fixed_BUILD_for_sets.bazel vendor/k8s.io/apimachinery/pkg/util/sets/BUILD
	go build -o build/bin/buildozer vendor/github.com/bazelbuild/buildtools/buildozer/*.go
	rm -rf vendor/github.com/bazelbuild
	bazel run //:gazelle_fix

.PHONY: update-bazel
update-bazel:
	-build/bin/buildozer 'set race "on"' \
		//cmd/...:%go_test \
		//examples/...:%go_test \
		//it/...:%go_test \
		//pkg/...:%go_test
	bazel run //:gazelle

.PHONY: build
build: fmt update-bazel build-ci

# Commented out for now. --features=race creates undesired side effects in the build
#.PHONY: build-race
#build-race: fmt update-bazel
#	bazel build --features=race //cmd/smith

.PHONY: build-ci
build-ci:
	bazel build //cmd/smith

.PHONY: fmt
fmt:
	goimports -w=true -d $(ALL_GO_FILES)

.PHONY: print-bundle-schema
print-bundle-schema:
	bazel run //cmd/schema -- -print-bundle-schema=yaml

.PHONY: generate
generate: generate-client generate-deepcopy

.PHONY: generate-client
generate-client:
	bazel build //vendor/k8s.io/code-generator/cmd/client-gen
	# Generate the versioned clientset (pkg/client/clientset_generated/clientset)
	bazel-bin/vendor/k8s.io/code-generator/cmd/client-gen/$(BINARY_PREFIX_DIRECTORY)/client-gen $(VERIFY_CODE) \
	--input-base "github.com/atlassian/smith/pkg/apis/" \
	--input "smith/v1" \
	--clientset-path "github.com/atlassian/smith/pkg/client/clientset_generated/" \
	--clientset-name "clientset" \
	--go-header-file "build/code-generator/boilerplate.go.txt"

.PHONY: generate-deepcopy
generate-deepcopy:
	bazel build //vendor/k8s.io/code-generator/cmd/deepcopy-gen
	# Generate deep copies
	bazel-bin/vendor/k8s.io/code-generator/cmd/deepcopy-gen/$(BINARY_PREFIX_DIRECTORY)/deepcopy-gen $(VERIFY_CODE) \
	--go-header-file "build/code-generator/boilerplate.go.txt" \
	--input-dirs "github.com/atlassian/smith/pkg/apis/smith/v1,github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1" \
	--bounding-dirs "github.com/atlassian/smith/pkg/apis/smith/v1,github.com/atlassian/smith/examples/sleeper/pkg/apis/sleeper/v1" \
	--output-file-base zz_generated.deepcopy

.PHONY: minikube-test
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

.PHONY: minikube-test-sc
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

.PHONY: minikube-run
minikube-run: fmt update-bazel build
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel-bin/cmd/smith/$(BINARY_PURE_PREFIX_DIRECTORY)/smith -disable-service-catalog -leader-elect

.PHONY: minikube-run-sc
minikube-run-sc: fmt update-bazel build
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel-bin/cmd/smith/$(BINARY_PURE_PREFIX_DIRECTORY)/smith  \
	-leader-elect \
	-service-catalog-url="https://$$(minikube ip):30443" \
	-service-catalog-insecure

.PHONY: minikube-sleeper-run
minikube-sleeper-run: fmt update-bazel
	bazel build //examples/sleeper/main
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	bazel-bin/examples/sleeper/main/$(BINARY_PURE_PREFIX_DIRECTORY)/main

.PHONY: test
test: fmt update-bazel test-ci

.PHONY: verify
verify:
	VERIFY_CODE=--verify-only make generate
	# TODO verify BUILD.bazel files are up to date

.PHONY: test-ci
test-ci:
	# TODO: why does it build binaries and docker in cmd?
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- //... -//cmd/... -//vendor/...

.PHONY: check
check:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... \
		--vendor --skip=pkg/client/clientset_generated --exclude=zz_generated \
		--linter='errcheck:errcheck:-ignore=net:Close' --cyclo-over=20 \
		--disable=interfacer --disable=golint --dupl-threshold=200

.PHONY: check-all
check-all:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor --cyclo-over=20 \
		--dupl-threshold=65

.PHONY: docker
docker:
	bazel build --cpu=k8 //cmd/smith:container

# Export docker image into local Docker
.PHONY: docker-export
docker-export:
	bazel run --cpu=k8 //cmd/smith:container -- --norun

.PHONY: release
release: update-bazel
	bazel run --cpu=k8 //cmd/smith:push_docker
