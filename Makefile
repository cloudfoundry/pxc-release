GINKGO = go run github.com/onsi/ginkgo/v2/ginkgo
GINKGO_OPTS ?= -v --procs=3

help:  ## Print this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[\/a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

show-labels-for-e2e-tests:  ## Print valid labels for filtering e2e-tests
	@cd src/e2e-tests && $(GINKGO) labels

e2e-tests:  ## Run e2e-tests; Optionally specify LABEL=query to filter tests
	./scripts/create-and-upload-dev-release
	./scripts/test-e2e $(GINKGO_OPTS) $(if $(LABEL),--label-filter=$(LABEL))

unit-tests: ## Run unit tests
	./scripts/test-unit

integration-tests: ## Run integration tests
	./scripts/test-integration

clean-bosh:  ## Cleanup BOSH dev deployments and orphaned disks
	$(MAKE) delete-dev-pxc-deployments
	$(MAKE) delete-orphaned-disks

delete-dev-pxc-deployments:  ## Delete BOSH deployments starting with "pxc-" prefix
	bosh deployments --json \
	| yq '.Tables[].Rows[].name|select(test("pxc-"))' \
	| xargs -P4 -n 1 -t bosh -n delete-deployment --force -d

delete-orphaned-disks:  ## Delete all BOSH orphaned disks
	bosh disks --orphaned --json \
		| yq .Tables[].Rows[].disk_cid \
		| xargs -n1 -t -P4 bosh -n delete-disk
