.PHONY: fmt fmt-check fix website-dev website-check website-deploy

fmt:
	golangci-lint fmt .

fmt-check:
	golangci-lint fmt --diff .

fix:
	go fix -omitzero=false ./...
	$(MAKE) fmt

website-dev:
	cd website && npm run dev

website-check:
	cd website && npm run check && npm test && npm run build

website-deploy:
	gh workflow run website.yml --ref main
	@echo "Deploy started. Follow it with: gh run watch"
