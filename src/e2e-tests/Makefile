GINKGO_OPTS ?= --always-emit-ginkgo-writer --procs=3
TOPDIR = $(shell git rev-parse --show-toplevel)

test:
	cd "$(TOPDIR)" && ./scripts/create-and-upload-dev-release
	go run github.com/onsi/ginkgo/v2/ginkgo $(GINKGO_OPTS)

delete-pxc-deployments:
	bosh deployments --json \
	| yq '.Tables[].Rows[].name|select(test("pxc-"))' \
	| xargs -P4 -n 1 -t bosh -n delete-deployment -d

delete-orphaned-disks:
	bosh disks --orphaned --json \
		| yq .Tables[].Rows[].disk_cid \
		| xargs -n1 -t -P4 bosh -n delete-disk

clean:
	$(MAKE) delete-pxc-deployments
	$(MAKE) delete-orphaned-disks
