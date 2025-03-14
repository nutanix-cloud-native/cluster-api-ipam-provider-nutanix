# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

INCLUDE_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

include $(INCLUDE_DIR)make.mk
include $(INCLUDE_DIR)shell.mk
include $(INCLUDE_DIR)help.mk
include $(INCLUDE_DIR)repo.mk
include $(INCLUDE_DIR)pre-commit.mk
include $(INCLUDE_DIR)go.mk
include $(INCLUDE_DIR)goreleaser.mk
include $(INCLUDE_DIR)tag.mk
include $(INCLUDE_DIR)kind.mk
include $(INCLUDE_DIR)clusterctl.mk
include $(INCLUDE_DIR)dev.mk
include $(INCLUDE_DIR)docs.mk
