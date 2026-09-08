DEV_KIT_VERSION := v1.0.13
-include common.mk
common.mk:
	@[ -f .common.mk-download ] || \
		curl --fail -sSL https://raw.githubusercontent.com/opendefensecloud/dev-kit/$(DEV_KIT_VERSION)/common.mk \
		  -o .common.mk-download
	mv .common.mk-download $@
	printf '%s' '$(DEV_KIT_VERSION)' > .common.mk-version
	touch .common.mk-checked

ENVTEST_K8S_VERSION ?= 1.37.0

export GOPRIVATE=*.go.opendefense.cloud/kit
export GNOSUMDB=*.go.opendefense.cloud/kit
export GNOPROXY=*.go.opendefense.cloud/kit

APISERVER_IMG ?= apiserver:latest
MANAGER_IMG ?= manager:latest

.PHONY: codegen
codegen: $(OPENAPI_GEN) ## Run code generation, e.g. openapi
	OPENAPI_GEN=$(OPENAPI_GEN) ./hack/update-codegen.sh

.PHONY: fmt
fmt: $(ADDLICENSE) ## Add license headers and format code
	find . -not -path '*/.*' -name '*.go' -exec $(ADDLICENSE) -c 'BWI GmbH and contributors' -l apache -s=only {} +
	$(GO) fmt ./...
	$(GOLANGCI_LINT) run --fix

.PHONY: lint
lint: lint-no-golangci $(GOLANGCI_LINT) ## Run linters such as golangci-lint and addlicence checks
	$(GOLANGCI_LINT) run -v

.PHONY: lint-no-golangci
lint-no-golangci: $(ADDLICENSE)
	find . -not -path '*/.*' -name '*.go' -exec $(ADDLICENSE) -check  -l apache -s=only -check {} +
	shellcheck hack/*.sh

.PHONY: test
test: $(SETUP_ENVTEST) $(GINKGO) ## Run all tests
	@KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) -r -cover --fail-fast --require-suite -covermode count --output-dir=$(BUILD_PATH) -coverprofile=example.full.coverprofile $(testargs)
	@cat example.full.coverprofile | grep -v /example/api > example.coverprofile
