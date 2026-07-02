# Copyright 2024 Nutanix. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables

AVAILABLE_PARALLELISM ?= $(shell sh -c 'p=$$(nproc --ignore=1 2>/dev/null); if [ -n "$$p" ]; then echo "$$p"; else n=$$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 2); if [ "$$n" -gt 1 ]; then echo $$((n - 1)); else echo 1; fi; fi')
