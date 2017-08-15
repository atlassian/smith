VERSION_VAR := main.Version
GIT_VAR := main.GitCommit
BUILD_DATE_VAR := main.BuildDate
REPO_VERSION := "0.0"
#REPO_VERSION = $$(git describe --abbrev=0 --tags)
BUILD_DATE := $$(date +%Y-%m-%d-%H:%M)
GIT_HASH := $$(git rev-parse --short HEAD)
GOBUILD_VERSION_ARGS := -ldflags "-s -X $(VERSION_VAR)=$(REPO_VERSION) -X $(GIT_VAR)=$(GIT_HASH) -X $(BUILD_DATE_VAR)=$(BUILD_DATE)"
BINARY_NAME := smith
IMAGE_NAME := atlassianlabs/smith
ARCH ?= darwin
METALINTER_CONCURRENCY ?= 4
GOVERSION := 1.8
GP := /gopath
GOPATH ?= "$$HOME/go"
MAIN_PKG := github.com/atlassian/smith/cmd/smith
ALL_GO_FILES=$$(find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./build/*" -not -path './pkg/client/clientset_generated/*' -not -name 'zz_generated.*')

setup: setup-ci
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install

setup-ci:
	go get -u github.com/golang/dep/cmd/dep
	go get -u github.com/Masterminds/glide
	dep ensure

build: fmt
	go build -i -o build/bin/$(ARCH)/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) $(MAIN_PKG)

build-race: fmt
	go build -i -race -o build/bin/$(ARCH)/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) $(MAIN_PKG)

build-all: fmt
	go install $$(glide nv | grep -v integration_tests | grep -v ./build/)
	go test -i -tags='integration integration_sc' $$(glide nv | grep -v ./build/)

build-all-race: fmt build-all-race-ci

build-all-race-ci:
	go install -race $$(glide nv | grep -v integration_tests | grep -v ./build/)
	go test -i -race -tags='integration integration_sc' $$(glide nv | grep -v ./build/)

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

minikube-test: fmt
	go test -i -tags=integration -race -v ./integration_tests
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	go test -tags=integration -race -v ./integration_tests

minikube-test-sc: fmt
	go test -i -tags=integration_sc -race -v ./integration_tests
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	SERVICE_CATALOG_URL="https://$$(minikube ip):30443" \
	go test -tags=integration_sc -race -v ./integration_tests

minikube-run: build-all-race
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	go run -race cmd/smith/*.go

minikube-run-sc: build-all-race
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	go run -race cmd/smith/*.go -service-catalog-url="https://$$(minikube ip):30443"

minikube-sleeper-run: build-all-race
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	KUBERNETES_SERVICE_HOST="$$(minikube ip)" \
	KUBERNETES_SERVICE_PORT=8443 \
	KUBERNETES_CA_PATH="$$HOME/.minikube/ca.crt" \
	KUBERNETES_CLIENT_CERT="$$HOME/.minikube/apiserver.crt" \
	KUBERNETES_CLIENT_KEY="$$HOME/.minikube/apiserver.key" \
	go run -race examples/sleeper/main/*.go

test: fmt test-ci

test-ci:
	go test -i -race $$(glide nv | grep -v integration_tests | grep -v ./build/)
	KUBE_PATCH_CONVERSION_DETECTOR=true \
	KUBE_CACHE_MUTATION_DETECTOR=true \
	go test -race $$(glide nv | grep -v integration_tests | grep -v ./build/)

check: build-all
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor \
		--linter='errcheck:errcheck:-ignore=net:Close' --cyclo-over=20 \
		--disable=interfacer --disable=golint --dupl-threshold=200

check-all: build-all
	gometalinter --concurrency=$(METALINTER_CONCURRENCY) --deadline=800s ./... --vendor --cyclo-over=20 \
		--dupl-threshold=65

coveralls:
	./cover.sh
	goveralls -coverprofile=coverage.out -service=travis-ci

# Compile a static binary. Cannot be used with -race
docker:
	docker pull golang:$(GOVERSION)
	docker run \
		--rm \
		-v "$(GOPATH)":"$(GP)" \
		-w "$(GP)/src/github.com/atlassian/smith" \
		-e GOPATH="$(GP)" \
		-e CGO_ENABLED=0 \
		golang:$(GOVERSION) \
		go build -o build/bin/linux/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) -installsuffix cgo $(MAIN_PKG)
	docker build --pull -t $(IMAGE_NAME):$(GIT_HASH) build

# Compile a binary with -race. Needs to be run on a glibc-based system.
docker-race:
	docker pull golang:$(GOVERSION)
	docker run \
		--rm \
		-v "$(GOPATH)":"$(GP)" \
		-w "$(GP)/src/github.com/atlassian/smith" \
		-e GOPATH="$(GP)" \
		golang:$(GOVERSION) \
		go build -race -o build/bin/linux/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) -installsuffix cgo $(MAIN_PKG)
	docker build --pull -t $(IMAGE_NAME):$(GIT_HASH)-race -f build/Dockerfile-glibc build

release-hash: docker
	docker push $(IMAGE_NAME):$(GIT_HASH)

release-normal: release-hash
#	docker tag $(IMAGE_NAME):$(GIT_HASH) $(IMAGE_NAME):latest
#	docker push $(IMAGE_NAME):latest
	docker tag $(IMAGE_NAME):$(GIT_HASH) $(IMAGE_NAME):$(REPO_VERSION)
	docker push $(IMAGE_NAME):$(REPO_VERSION)

release-hash-race: docker-race
	docker push $(IMAGE_NAME):$(GIT_HASH)-race

release-race: docker-race
	docker tag $(IMAGE_NAME):$(GIT_HASH)-race $(IMAGE_NAME):$(REPO_VERSION)-race
	docker push $(IMAGE_NAME):$(REPO_VERSION)-race

release: release-normal release-race

.PHONY: build
