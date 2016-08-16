VERSION_VAR := main.Version
GIT_VAR := main.GitCommit
BUILD_DATE_VAR := main.BuildDate
REPO_VERSION := "0.0"
#REPO_VERSION = $$(git describe --abbrev=0 --tags)
BUILD_DATE := $$(date +%Y-%m-%d-%H:%M)
GIT_HASH := $$(git rev-parse --short HEAD)
GOBUILD_VERSION_ARGS := -ldflags "-s -X $(VERSION_VAR)=$(REPO_VERSION) -X $(GIT_VAR)=$(GIT_HASH) -X $(BUILD_DATE_VAR)=$(BUILD_DATE)"
BINARY_NAME := smith
IMAGE_NAME := ash2k/smith
ARCH ?= darwin
GOVERSION := 1.7.0
GP := /gopath

setup-ci:
	go get -v -u github.com/Masterminds/glide
	glide install

build: *.go fmt
	go build -o build/bin/$(ARCH)/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) github.com/ash2k/smith

build-race: *.go fmt
	go build -race -o build/bin/$(ARCH)/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) github.com/ash2k/smith

build-all:
	go build $$(glide nv)

fmt:
	gofmt -w=true -s $$(find . -type f -name '*.go' -not -path "./vendor/*")
	goimports -w=true -d $$(find . -type f -name '*.go' -not -path "./vendor/*")

test:
	go test $$(glide nv)

test-race:
	go test -race $$(glide nv)

# Compile a static binary. Cannot be used with -race
docker:
	docker run \
		--rm \
		-v "$(GOPATH)":"$(GP)" \
		-w "$(GP)/src/github.com/ash2k/smith" \
		-e GOPATH="$(GP)" \
		-e CGO_ENABLED=0 \
		golang:$(GOVERSION) \
		go build -o build/bin/linux/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) -a -installsuffix cgo github.com/ash2k/smith
	docker build --pull -t $(IMAGE_NAME):$(GIT_HASH) build

# Compile a binary with -race. Needs to be run on a glibc-based system.
docker-race:
	docker run \
		--rm \
		-v "$(GOPATH)":"$(GP)" \
		-w "$(GP)/src/github.com/ash2k/smith" \
		-e GOPATH="$(GP)" \
		golang:$(GOVERSION) \
		go build -race -o build/bin/linux/$(BINARY_NAME) $(GOBUILD_VERSION_ARGS) -a -installsuffix cgo github.com/ash2k/smith
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
