OS := $(shell uname -s | tr A-Z a-z)
BINARY_PREFIX_DIRECTORY := $(OS)_amd64_stripped
KUBECONTEXT ?= kubernetes-admin@1
KUBECONFIG ?= $(shell kind get kubeconfig-path)
#KUBECONFIG ?= $$HOME/.kube/config

.PHONY: setup-dev
setup-dev: setup-ci

.PHONY: setup-ci
setup-ci: setup-base

.PHONY: setup-base
setup-base:
	dep ensure

.PHONY: fmt-bazel
fmt-bazel:
	bazel run //:buildozer
	bazel run //:buildifier

.PHONY: update-bazel
update-bazel:
	bazel run //:gazelle

.PHONY: fmt
fmt:
	bazel run //:goimports

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
		--test_env=KUBERNETES_CONFIG_FILENAME='$(KUBECONFIG)' \
		--test_env=KUBERNETES_CONFIG_CONTEXT='$(KUBECONTEXT)' \
		//it:go_default_test

.PHONY: integration-test-sc
integration-test-sc: fmt update-bazel integration-test-sc-ci

.PHONY: integration-test-sc-ci
integration-test-sc-ci:
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--test_env=KUBERNETES_CONFIG_FROM=file \
		--test_env=KUBERNETES_CONFIG_FILENAME='$(KUBECONFIG)' \
		--test_env=KUBERNETES_CONFIG_CONTEXT='$(KUBECONTEXT)' \
		//it/sc:go_default_test

.PHONY: run
run: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	bazel run //cmd/smith:smith_race \
		-- \
		-log-encoding=console \
		-bundle-service-catalog=false \
		-leader-elect \
		-client-config-from=file \
		-client-config-file-name='$(KUBECONFIG)' \
		-client-config-context='$(KUBECONTEXT)'

.PHONY: run-sc
run-sc: fmt update-bazel
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	bazel run //cmd/smith:smith_race \
		-- \
		-log-encoding=console \
		-leader-elect \
		-client-config-from=file \
		-client-config-file-name='$(KUBECONFIG)' \
		-client-config-context='$(KUBECONTEXT)'

.PHONY: sleeper-run
sleeper-run: fmt update-bazel
	bazel build //examples/sleeper/main:main_race
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	bazel run //examples/sleeper/main:main_race \
		-- \
		-client-config-from=file \
		-client-config-file-name='$(KUBECONFIG)' \
		-client-config-context='$(KUBECONTEXT)'

.PHONY: test
test: fmt update-bazel test-ci

.PHONY: verify
verify:
	bazel run //:buildifier_check
	VERIFY_CODE=--verify-only make generate
	# TODO verify BUILD.bazel files are up to date

.PHONY: test-ci
test-ci:
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		-- //... -//vendor/...
	bazel build $$(bazel query 'attr(tags, manual, kind(test, //... -//vendor/...))')

.PHONY: quick-test
quick-test:
	bazel test \
		--test_env=KUBE_PATCH_CONVERSION_DETECTOR=true \
		--test_env=KUBE_CACHE_MUTATION_DETECTOR=true \
		--build_tests_only \
		-- //... -//vendor/...

.PHONY: check
check:
	bazel run //:gometalinter

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
	bazel run \
		--platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 \
		//cmd/smith:push_docker_race
