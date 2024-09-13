API_NAME := api
VERSION ?= dev
COMMIT=$(shell git rev-parse --short HEAD)
BRANCH=$(shell git rev-parse --abbrev-ref HEAD)
TIME=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

.PHONY: config
config: ## Creating the local config .env
	@echo "Creating local config .env ..."
	cp .sample.env .env

.PHONY: api
api: ## Run service http api
	@echo "Running api..."
	go run cmd/$(API_NAME)/*.go