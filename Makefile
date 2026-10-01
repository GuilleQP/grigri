# Targets for the local Kind cluster only. Never point these at another context.
KUBE_CONTEXT ?= kind-kagent
REGISTRY     ?= localhost:5001
IMAGE_REPO   ?= $(REGISTRY)/grigri/kagent-harness
TAG          ?= dev
PLATFORM     ?= linux/arm64

KUBECTL = kubectl --context $(KUBE_CONTEXT)

.PHONY: build test image push digest deploy undeploy

build:
	go build -o bin/ ./...

test:
	go vet ./... && go test ./...

image:
	docker buildx build --platform $(PLATFORM) -t $(IMAGE_REPO):$(TAG) --load .

push: image
	docker push $(IMAGE_REPO):$(TAG)

# Prints the image pinned by digest (Harness images must use a digest).
digest:
	@echo $(IMAGE_REPO)@$$(docker buildx imagetools inspect $(IMAGE_REPO):$(TAG) --format '{{json .Manifest.Digest}}' | tr -d '"')

deploy: push
	IMAGE=$$($(MAKE) -s digest) envsubst '$${IMAGE}' < deploy/byo-example/example.yaml | $(KUBECTL) apply -f -
	$(KUBECTL) apply -f deploy/byo-example/playground.yaml

undeploy:
	$(KUBECTL) delete -f deploy/byo-example/example.yaml --ignore-not-found
	$(KUBECTL) delete -f deploy/byo-example/playground.yaml --ignore-not-found
