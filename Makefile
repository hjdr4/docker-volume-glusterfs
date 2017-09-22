VERSION=0.2

################################################################################


BUILD               = $(shell git rev-parse HEAD)

PLATFORMS           = linux_amd64 linux_386 linux_arm darwin_amd64 darwin_386 freebsd_amd64 freebsd_386 windows_386 windows_amd64

FLAGS_all           = GOPATH=$(GOPATH)
FLAGS_linux_amd64   = $(FLAGS_all) GOOS=linux GOARCH=amd64
FLAGS_linux_386     = $(FLAGS_all) GOOS=linux GOARCH=386
FLAGS_linux_arm     = $(FLAGS_all) GOOS=linux GOARCH=arm
FLAGS_darwin_amd64  = $(FLAGS_all) GOOS=darwin GOARCH=amd64
FLAGS_darwin_386    = $(FLAGS_all) GOOS=darwin GOARCH=386
FLAGS_freebsd_amd64 = $(FLAGS_all) GOOS=freebsd GOARCH=amd64
FLAGS_freebsd_386   = $(FLAGS_all) GOOS=freebsd GOARCH=386
FLAGS_windows_386   = $(FLAGS_all) GOOS=windows GOARCH=386
FLAGS_windows_amd64 = $(FLAGS_all) GOOS=windows GOARCH=amd64

EXTENSION_windows_386=.exe
EXTENSION_windows_amd64=.exe

msg=@printf "\n\033[0;01m>>> %s\033[0m\n" $1


################################################################################


.DEFAULT_GOAL := build

build: guard-VERSION deps
	$(call msg,"Build binary")
	$(FLAGS_all) go build -ldflags "-X main.Version=${VERSION} -X main.Build=${BUILD}" -o docker-volume-glusterfs$(EXTENSION_$GOOS_$GOARCH) *.go
	./docker-volume-glusterfs -version
.PHONY: build

deps:
	$(call msg,"Get dependencies")
	go get -t ./...
.PHONY: deps

plugin: guard-IMAGE build
	mkdir -p content
	cp -f docker-volume-glusterfs content/
	docker build -t ${IMAGE} .
	$(eval ID:=$(shell docker create ${IMAGE} true)) 
	mkdir -p plugin/rootfs
	docker export $(ID) | sudo tar -x -C plugin/rootfs
	cp config.json plugin/

plugin-install: guard-IMAGE guard-SERVERS
	sudo docker plugin create ${IMAGE} plugin/	
	docker plugin set ${IMAGE} args="-servers=${SERVERS}"
	docker plugin enable ${IMAGE}


test: deps
	$(call msg,"Run tests")
	$(FLAGS_all) go test $(wildcard ../*.go)
.PHONY: test

clean: guard-IMAGE
	$(call msg,"Clean directory")
	rm -f docker-volume-glusterfs
	rm -rf content
	sudo rm -rf plugin
	docker plugin disable ${IMAGE} -f || true
	docker plugin rm ${IMAGE} -f || true
.PHONY: clean

release: guard-VERSION dist
	$(call msg,"Create and push release")
	git tag -a "v$(VERSION)" -m "Release $(VERSION)"
	git push --tags
.PHONY: tag-release


################################################################################

guard-%:
	@ if [ "${${*}}" = "" ]; then \
		echo "Environment variable $* not set"; \
		exit 1; \
	fi


################################################################################
