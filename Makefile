VERSION			?= $(shell git describe 2>/dev/null || echo "v0.$(git rev-list --count HEAD)")
AUTHOR			:= ashald
PLUGIN_NAME		:= docker-volume-loopback
PLUGIN_FULL_NAME	:= ${AUTHOR}/${PLUGIN_NAME}
ROOTFS_CONTAINER	:= ${PLUGIN_NAME}-rootfs
ROOTFS_IMAGE		:= ${AUTHOR}/${ROOTFS_CONTAINER}
# the build host's architecture: inside the Dockerfile's builder that is the Docker host's, so
# `make plugin` on an arm64 host gives an arm64 plugin
GOARCH			?= $(shell go env GOARCH 2>/dev/null || echo amd64)


.PHONY: all
all: format build


.PHONY: format
format:
	go fmt ./...


.PHONY: build
build:
	GOOS=linux GOARCH=$(GOARCH) go build -o $(PLUGIN_NAME)


.PHONY: test
test:
	./tests/run.sh


.PHONY: clean
clean:
	mkdir -p ./plugin/rootfs
	rm -rf ./plugin/rootfs
	rm -f ./$(ROOTFS_IMAGE)


.PHONY: clean-plugin
clean-plugin:
	docker plugin rm --force $(PLUGIN_FULL_NAME):$(VERSION) || true
	docker plugin rm --force $(PLUGIN_FULL_NAME) || true


.PHONY: rootfs-image
rootfs-image:
	docker build -t $(ROOTFS_IMAGE) .


.PHONY: rootfs
rootfs: rootfs-image
	docker rm -vf $(ROOTFS_CONTAINER) || true
	docker create --name $(ROOTFS_CONTAINER) $(ROOTFS_IMAGE) || true
	mkdir -p ./plugin/rootfs
	rm -rf ./plugin/rootfs/*
	docker export $(ROOTFS_CONTAINER) | tar -x -C ./plugin/rootfs
	docker rm -vf $(ROOTFS_CONTAINER)


.PHONY: plugin
plugin: rootfs
	docker plugin disable $(PLUGIN_NAME) || true
	docker plugin rm --force $(PLUGIN_NAME) || true
	docker plugin create $(PLUGIN_NAME) ./plugin
	docker plugin enable $(PLUGIN_NAME)


.PHONY: plugin-push-version
plugin-push-version: rootfs
	docker plugin rm --force $(PLUGIN_FULL_NAME):$(VERSION) || true
	docker plugin create $(PLUGIN_FULL_NAME):$(VERSION) ./plugin
	docker plugin push $(PLUGIN_FULL_NAME):$(VERSION)


.PHONY: plugin-push-default
plugin-push-default: rootfs
	docker plugin rm --force $(PLUGIN_FULL_NAME) || true
	docker plugin create $(PLUGIN_FULL_NAME) ./plugin
	docker plugin push $(PLUGIN_FULL_NAME)


.PHONY: plugin-push
plugin-push: plugin-push-version plugin-push-default
