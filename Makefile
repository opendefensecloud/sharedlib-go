# Include ODC common make targets
DEV_KIT_VERSION := v2.0.0
DEV_KIT_VERSION := v2.1.0
-include common.mk
common.mk:
	@[ -f .common.mk-download ] || \
		curl --fail -sSL https://raw.githubusercontent.com/opendefensecloud/dev-kit/$(DEV_KIT_VERSION)/common.mk \
		  -o .common.mk-download
	mv .common.mk-download $@
	printf '%s' '$(DEV_KIT_VERSION)' > .common.mk-version
	touch .common.mk-checked

HACK_DIR ?= $(shell cd hack 2>/dev/null && pwd)

ENVTEST_K8S_VERSION ?= 1.37.0

export GOPRIVATE=*.go.opendefense.cloud/kit/
export GNOSUMDB=*.go.opendefense.cloud/kit/
export GNOPROXY=*.go.opendefense.cloud/kit/

LICENSE := apache
LICENSE_COMMENT := BWI GmbH and contributors

.PHONY: fmt
fmt: $(GOLANGCI_LINT) ## Run formatters
	$(MAKE) addlicense license=$(LICENSE) comment='$(LICENSE_COMMENT)' pattern='*\.go'
	$(GO) fmt ./...
	$(GOLANGCI_LINT) run --fix

.PHONY: lint
lint: lint-no-golangci golangci-lint ## Run linters

.PHONY: lint-no-golangci
lint-no-golangci: shellcheck ## Run linters but not golangci-lint to exit early in CI/CD pipeline
	$(MAKE) addlicense-check license=apache comment='$(LICENSE_COMMENT)' pattern='*\.go'

.PHONY: test
test: $(SETUP_ENVTEST) $(GINKGO) envtest-binaries-sideload ## Run all tests
	@KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -i -p path)" \
	$(GINKGO) -r -cover --fail-fast --require-suite -covermode count --output-dir=$(BUILD_PATH) -coverprofile=sl.coverprofile --skip-package ./example/bin/ $(testargs)
