# cguard 跨平台构建与打包
# 详见 docs/BUILD.md

APP_NAME   := cguard
MAIN_PKG   := .

DIST_DIR   := dist
BUILD_DIR  := $(DIST_DIR)/build

GO         ?= go
CGO_ENABLED ?= 0

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

GO_BUILD_FLAGS := -trimpath -ldflags "-s -w"

LINUX_BIN   := $(BUILD_DIR)/$(APP_NAME)-linux-amd64/$(APP_NAME)
WINDOWS_BIN := $(BUILD_DIR)/$(APP_NAME)-windows-amd64/$(APP_NAME).exe

LINUX_ARCHIVE   := $(DIST_DIR)/$(APP_NAME)-$(VERSION)-linux-amd64.tar.gz
WINDOWS_ARCHIVE := $(DIST_DIR)/$(APP_NAME)-$(VERSION)-windows-amd64.zip

PACKAGE_FILES := config.yaml README.md

.PHONY: all help build linux windows package package-linux package-windows clean

all: package

help:
	@echo "cguard $(VERSION) — 跨平台构建"
	@echo ""
	@echo "  make build          编译 linux/amd64 与 windows/amd64"
	@echo "  make linux          仅编译 Linux amd64"
	@echo "  make windows        仅编译 Windows amd64"
	@echo "  make package        编译并打包（默认 all）"
	@echo "  make package-linux  打包 Linux 发布包 (.tar.gz)"
	@echo "  make package-windows 打包 Windows 发布包 (.zip)"
	@echo "  make clean          清理 $(DIST_DIR)/"
	@echo ""
	@echo "产物目录: $(DIST_DIR)/"

build: linux windows

linux: $(LINUX_BIN)

windows: $(WINDOWS_BIN)

$(LINUX_BIN):
	@mkdir -p $(dir $@)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 \
		$(GO) build $(GO_BUILD_FLAGS) -o $@ $(MAIN_PKG)

$(WINDOWS_BIN):
	@mkdir -p $(dir $@)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 \
		$(GO) build $(GO_BUILD_FLAGS) -o $@ $(MAIN_PKG)

package: package-linux package-windows

package-linux: linux $(LINUX_ARCHIVE)

package-windows: windows $(WINDOWS_ARCHIVE)

# staging: dist/staging/<platform>/{binary,config.yaml,README.md}
LINUX_STAGE   := $(DIST_DIR)/staging/linux-amd64
WINDOWS_STAGE := $(DIST_DIR)/staging/windows-amd64

$(LINUX_STAGE)/$(APP_NAME): $(LINUX_BIN)
	@rm -rf $(LINUX_STAGE)
	@mkdir -p $(LINUX_STAGE)
	cp $(LINUX_BIN) $(LINUX_STAGE)/$(APP_NAME)
	@for f in $(PACKAGE_FILES); do cp $$f $(LINUX_STAGE)/; done

$(WINDOWS_STAGE)/$(APP_NAME).exe: $(WINDOWS_BIN)
	@rm -rf $(WINDOWS_STAGE)
	@mkdir -p $(WINDOWS_STAGE)
	cp $(WINDOWS_BIN) $(WINDOWS_STAGE)/$(APP_NAME).exe
	@for f in $(PACKAGE_FILES); do cp $$f $(WINDOWS_STAGE)/; done

$(LINUX_ARCHIVE): $(LINUX_STAGE)/$(APP_NAME)
	@mkdir -p $(DIST_DIR)
	tar -czf $@ -C $(DIST_DIR)/staging linux-amd64

$(WINDOWS_ARCHIVE): $(WINDOWS_STAGE)/$(APP_NAME).exe
	@mkdir -p $(DIST_DIR)
	cd $(DIST_DIR)/staging && zip -rq ../$(notdir $@) windows-amd64

clean:
	rm -rf $(DIST_DIR)
