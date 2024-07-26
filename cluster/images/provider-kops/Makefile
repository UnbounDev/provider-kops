# ====================================================================================
# Setup Project

include ../../../build/makelib/common.mk

# ====================================================================================
#  Options

include ../../../build/makelib/imagelight.mk

# ====================================================================================
# Targets

KOPS_VERSION    := v1.29.2
KUBECTL_VERSION := v1.30.2

img.build:
	@$(INFO) docker build $(IMAGE)
	@$(MAKE) BUILD_ARGS="--load" img.build.shared
	@$(OK) docker build $(IMAGE)

img.publish:
	@$(INFO) Skipping image publish for $(IMAGE)
	@echo Publish is deferred to xpkg machinery
	@$(OK) Image publish skipped for $(IMAGE)

img.build.shared:
ifeq ("$(wildcard $(OUTPUT_DIR)/bin/kubectl)","")
	curl -L https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/linux/amd64/kubectl \
	  -o $(OUTPUT_DIR)/bin/kubectl \
	  || $(FAIL)
	chmod +x $(OUTPUT_DIR)/bin/kubectl || $(FAIL)
endif
ifeq ("$(wildcard $(OUTPUT_DIR)/bin/kops)","")
	curl -L https://github.com/kubernetes/kops/releases/download/$(KOPS_VERSION)/kops-linux-amd64 \
	  -o $(OUTPUT_DIR)/bin/kops \
	  || $(FAIL)
	chmod +x $(OUTPUT_DIR)/bin/kops || $(FAIL)
endif
	@cp Dockerfile $(IMAGE_TEMP_DIR) || $(FAIL)
	@cp -r $(OUTPUT_DIR)/bin/ $(IMAGE_TEMP_DIR)/bin || $(FAIL)
	@docker buildx build $(BUILD_ARGS) \
		--platform $(IMAGE_PLATFORMS) \
		-t $(IMAGE) \
		$(IMAGE_TEMP_DIR) || $(FAIL)

img.promote:
	@$(INFO) Skipping image promotion from $(FROM_IMAGE) to $(TO_IMAGE)
	@echo Promote is deferred to xpkg machinery
	@$(OK) Image promotion skipped for $(FROM_IMAGE) to $(TO_IMAGE)
