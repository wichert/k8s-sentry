NAME := helm-k8s-sentry
HELM_VERSION := 3.3.4
HELM_IN_DOCKER := docker run --rm -v ${PWD}:/apps alpine/helm:$(HELM_VERSION)

helm_lint:
	$(HELM_IN_DOCKER) lint ./$(NAME)

helm_template:
	$(HELM_IN_DOCKER) template ./$(NAME)

helm_package:
	$(HELM_IN_DOCKER) package ./$(NAME) --destination docs
	$(HELM_IN_DOCKER) repo index docs --url https://daaain.github.io/k8s-sentry

.PHONY: helm_lint helm_template helm_package


