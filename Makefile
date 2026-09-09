.PHONY: fmt fmt-check fix

fmt:
	golangci-lint fmt .

fmt-check:
	golangci-lint fmt --diff .

fix:
	go fix -omitzero=false ./...
	$(MAKE) fmt
