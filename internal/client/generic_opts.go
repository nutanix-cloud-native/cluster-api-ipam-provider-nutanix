// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

type AsyncTaskOpts struct {
	RequestID string

	WaitForTaskCompletionOpts
}
