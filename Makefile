GOCMD ?= go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
BINARY_NAME = rslb
BINARY_UNIX = $(BINARY_NAME)_unix

DEV_OUTPUT:=$(CURDIR)/$(BINARY_NAME)
DEPLOY_DIR:=$(CURDIR)/deploy
DEPLOY_OUTPUT:=$(DEPLOY_DIR)/$(BINARY_NAME)
UNAME_S=$(shell uname -s)

BUILD_ENV=GOTRACEBACK=all
BUILD_FLAG=--ldflags "-X main.Version=`date +.%Y%m%d.%H%M%S` -X main.Hostname=`hostname` -X main.BuildType=$@"

ifeq ($(UNAME_S), Linux)
	MD5_TOOL:=md5sum
endif

ifeq ($(UNAME_S), Darwin)
	MD5_TOOL:=md5 -r
endif

default: build

all: test build

build:
	env $(BUILD_ENV) $(GOBUILD) $(BUILD_FLAG) -o $(BINARY_NAME) -v && find . -type f -not -path '*/\.*' -exec $(MD5_TOOL) {} + >md5.release
test:
	$(GOTEST) -v ./..

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run: build
	./$(BINARY_NAME)

build-linux:
	CGO_ENABLE=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAG) -o $(BINARY_UNIX) -v

release:
	env GOOS=linux GOARCH=amd64 $(BUILD_ENV) $(GOCMD) build $(BUILD_FLAG) -o $(DEPLOY_OUTPUT) && find . -type f -not -path '*/\.*' -exec $(MD5_TOOL) {} + > $(DEPLOY_DIR)/md5.release