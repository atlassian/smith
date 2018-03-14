METALINTER_CONCURRENCY ?= 4
ALL_GO_FILES=$$(find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./build/*" -not -path './pkg/client/clientset_generated/*' -not -name 'zz_generated.*')
OS = $$(uname -s | tr A-Z a-z)
BINARY_PREFIX_DIRECTORY=$(OS)_amd64_stripped
KUBE_CONTEXT ?= minikube

.PHONY: setup-dev
setup-dev: setup-base
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install

.PHONY: setup-base
setup-base:
	dep ensure
	bazel run //:gazelle_fix

.PHONY: fmt-bazel
fmt-bazel:
	bazel build //vendor/github.com/bazelbuild/buildtools/buildifier //vendor/github.com/bazelbuild/buildtools/buildozer
	-bazel-bin/vendor/github.com/bazelbuild/buildtools/buildozer/$(BINARY_PREFIX_DIRECTORY)/buildozer \
		'set race "on"' \
		//:%go_test \
		//cmd/...:%go_test \
		//examples/...:%go_test \
		//it/...:%go_test \
		//pkg/...:%go_test
	find . -not -path "./vendor/*" -and \( -name '*.bzl' -or -name 'BUILD.bazel' -or -name 'WORKSPACE' \) -exec \
		bazel-bin/vendor/github.com/bazelbuild/buildtools/buildifier/$(BINARY_PREFIX_DIRECTORY)/buildifier {} +

.PHONY: update-bazel
update-bazel:
	bazel run //:gazelle

.PHONY: build
build: fmt update-bazel build-ci

.PHONY: build-ci
build-ci:
	bazel build //cmd/smith

.PHONY: fmt
fmt:
	goimports -w=true -d $(ALL_GO_FILES)

.PHONY: print-bundle-crd
print-bundle-crd: fmt update-bazel
	bazel run //cmd/crd -- -print-bundle=yaml

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

.PHONY: integration-test
integration-test: fmt update-bazel integration-test-ci

.PHONY: integration-test-ci
integration-test-ci:
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_CONFIG_FROM=file \
		--test_env=KUBERNETES_CONFIG_FILENAME=$$HOME/.kube/config \
		--test_env=KUBERNETES_CONFIG_CONTEXT=$(KUBE_CONTEXT) \
		//it:go_default_test

.PHONY: integration-test-sc
integration-test-sc: fmt update-bazel integration-test-sc-ci

.PHONY: integration-test-sc-ci
integration-test-sc-ci:
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_CONFIG_FROM=file \
		--test_env=KUBERNETES_CONFIG_FILENAME="$$HOME/.kube/config" \
		--test_env=KUBERNETES_CONFIG_CONTEXT=$(KUBE_CONTEXT) \
		//it/sc:go_default_test

.PHONY: run
run: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	build/bazel-run.sh //cmd/smith:smith_race \
		-- \
		-log-encoding=console \
		-service-catalog=false \
		-leader-elect \
		-client-config-from=file \
		-client-config-file-name="$$HOME/.kube/config" \
		-client-config-context=$(KUBE_CONTEXT)

.PHONY: run-sc
run-sc: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	build/bazel-run.sh //cmd/smith:smith_race \
		-- \
		-log-encoding=console \
		-log-level=debug \
		-leader-elect \
		-client-config-from=file \
		-client-config-file-name="$$HOME/.kube/config" \
		-client-config-context=$(KUBE_CONTEXT)

.PHONY: sleeper-run
sleeper-run: fmt update-bazel
	bazel build //examples/sleeper/main:main_race
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	build/bazel-run.sh //examples/sleeper/main:main_race \
		-- \
		-client-config-from=file \
		-client-config-file-name="$$HOME/.kube/config" \
		-client-config-context=$(KUBE_CONTEXT)

.PHONY: test
test: fmt update-bazel test-ci

.PHONY: verify
verify:
	bazel build //vendor/github.com/bazelbuild/buildtools/buildifier
	find . -not -path "./vendor/*" -and \( -name '*.bzl' -or -name 'BUILD.bazel' -or -name 'WORKSPACE' \) -exec \
		bazel-bin/vendor/github.com/bazelbuild/buildtools/buildifier/$(BINARY_PREFIX_DIRECTORY)/buildifier -showlog -mode=check {} +
	VERIFY_CODE=--verify-only make generate
	# TODO verify BUILD.bazel files are up to date

.PHONY: test-ci
test-ci:
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- //... -//vendor/...

.PHONY: check
check:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... \
		--vendor --skip=pkg/client/clientset_generated --exclude=zz_generated \
		--linter='errcheck:errcheck:-ignore=net:Close' --cyclo-over=30 \
		--disable=interfacer --disable=golint --dupl-threshold=200

.PHONY: check-all
check-all:
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... \
		--vendor --skip=pkg/client/clientset_generated --exclude=zz_generated \
		--cyclo-over=30 --dupl-threshold=65

.PHONY: docker
docker:
	bazel build \
		--platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 \
		//cmd/smith:container

# Export docker image into local Docker
.PHONY: docker-export
docker-export:
	bazel run \
		--platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 \
		//cmd/smith:container \
		-- \
		--norun

.PHONY: release
release: update-bazel
	bazel run \
		--platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 \
		//cmd/smith:push_docker
