# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

.PHONY: tag
tag:
ifndef NEW_GIT_TAG
	$(error Please specify git tag to create via NEW_GIT_TAG env var or make variable)
endif
	$(foreach module,\
		$(dir $(GO_SUBMODULES_NO_DOCS)),\
		git tag -s "$(module)$(NEW_GIT_TAG)" -a -m "$(module)$(NEW_GIT_TAG)";\
	)
	git tag -s "$(NEW_GIT_TAG)" -a -m "$(NEW_GIT_TAG)"
