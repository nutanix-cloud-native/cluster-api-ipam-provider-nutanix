// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	prismcommonapi "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/common/v1/config"
	prismapi "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/nutanix-cloud-native/prism-go-client/utils"
	v4 "github.com/nutanix-cloud-native/prism-go-client/v4"
)

const (
	defaultTaskPollInterval  = 100 * time.Millisecond
	defaultTaskTimeout       = 5 * time.Minute
	defaultTaskPollImmediate = false
)

type WaitForTaskCompletionOpts struct {
	PollInterval  time.Duration
	Timeout       time.Duration
	PollImmediate bool
}

type PrismClient interface {
	WaitForTaskCompletion(
		ctx context.Context,
		taskID string,
		opts WaitForTaskCompletionOpts,
	) ([]prismcommonapi.KVPair, error)
}

func (c *client) Prism() PrismClient {
	return &prismClient{v4Client: c.v4Client}
}

type prismClient struct {
	v4Client *v4.Client
}

func (p *prismClient) WaitForTaskCompletion(
	ctx context.Context,
	taskID string,
	opts WaitForTaskCompletionOpts,
) ([]prismcommonapi.KVPair, error) {
	pollInterval := defaultTaskPollInterval
	if opts.PollInterval != 0 {
		pollInterval = opts.PollInterval
	}
	timeout := defaultTaskTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	var data []prismcommonapi.KVPair

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := wait.PollUntilContextCancel(
		timeoutCtx,
		pollInterval,
		opts.PollImmediate,
		func(ctx context.Context) (done bool, err error) {
			task, err := p.v4Client.TasksApiInstance.GetTaskById(utils.StringPtr(taskID))
			if err != nil {
				return false, fmt.Errorf("failed to get task %s: %w", taskID, err)
			}

			taskData, ok := task.GetData().(prismapi.Task)
			if !ok {
				return false, fmt.Errorf("unexpected task data type %[1]T: %+[1]v", task.GetData())
			}

			if taskData.Status == nil {
				return false, nil
			}

			switch *taskData.Status {
			case prismapi.TASKSTATUS_SUCCEEDED:
				data = taskData.CompletionDetails
				return true, nil
			case prismapi.TASKSTATUS_FAILED, prismapi.TASKSTATUS_CANCELED:
				marshaled, _ := json.Marshal(taskData)
				return false, fmt.Errorf("task %s %s: %s", taskID, taskData.Status.GetName(), string(marshaled))
			default:
				return false, nil
			}
		},
	); err != nil {
		return nil, fmt.Errorf("failed to wait for task %s to complete: %w", taskID, err)
	}

	return data, nil
}
