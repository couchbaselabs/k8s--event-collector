BUILDDIR := build/bin
ARTIFACTS = build/artifacts/k8s-event-collector
VERSION ?= 0.1.0
bldNum = $(if $(BLD_NUM),$(BLD_NUM),9999)
REVISION := $(shell git rev-parse HEAD)
version = $(if $(VERSION),$(VERSION),1.0.0)
productVersion = $(version)-$(bldNum)


LDFLAGS = \
  -s -w \
  -X github.com/couchbase/k8s-event-collector/pkg/version.Version=$(version) \
  -X github.com/couchbase/k8s-event-collector/pkg/version.BuildNumber=$(bldNum) \
  -X github.com/couchbase/k8s-event-collector/pkg/revision.gitRevision=$(REVISION)

.PHONY: all
all: bins images

.PHONY:
images: bins
	docker buildx build --platform linux/arm64 --build-arg TARGET=kubernetes-linux-arm64 --load --build-arg GO_VERSION=1.20.8 --build-arg VERSION=2.6.0 --build-arg BLD_NUM=999 -t couchbase/event-collector:$(VERSION) .


.PHONY: bins
bins: | $(BUILDDIR)
	for platform in linux; do \
	    for arch in amd64 arm64; do \
		   echo "Building $$platform $$arch binary" ; \
		   CGO_ENABLED=0 GO111MODULE=on GOOS=$$platform GOARCH=$$arch go build -ldflags="$(LDFLAGS)" -o build/bin/$$platform/k8s-event-collector-$$arch cmd/event-collector/main.go ; \
		done \
	done

.PHONY: kind-images
kind-images: images
	kind load docker-image couchbase/event-collector:$(VERSION)

$(BUILDDIR):
	mkdir -p $(BUILDDIR)

image-artifacts: bins
	mkdir -p $(ARTIFACTS)/bin/linux
	cp build/bin/linux/k8s-event-collector-* $(ARTIFACTS)/bin/linux
	cp Dockerfile* License.txt README.md $(ARTIFACTS)

dist: image-artifacts
	    rm -rf dist
		mkdir -p dist
		tar -C $(ARTIFACTS)/.. -czf dist/k8s-event-collector-image_$(productVersion).tgz .