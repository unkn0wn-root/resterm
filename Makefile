.PHONY: fmt fmt-check fix site-dev site-check site-deploy

fmt:
	golangci-lint fmt .

fmt-check:
	golangci-lint fmt --diff .

fix:
	go fix -omitzero=false ./...
	$(MAKE) fmt

site-dev:
	cd site && npm run dev

site-check:
	cd site && npm run check && npm test && npm run build

site-deploy:
	gh workflow run site.yml --ref main
	@echo "Deploy started. Follow it with: gh run watch"
