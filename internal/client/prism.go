// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"fmt"

	prismcommonapi "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/common/v1/config"
	prismapi "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"

	"github.com/nutanix-cloud-native/prism-go-client/utils"
	v4 "github.com/nutanix-cloud-native/prism-go-client/v4"
)

const (
	// RequestIDHeaderName is the name of the header that carries the request ID.
	RequestIDHeaderName = "NTNX-Request-Id"
)

var (
	// ErrTaskFailed is returned when a task has failed.
	ErrTaskFailed = fmt.Errorf("task failed")
	// ErrTaskCancelled is returned when a task has been cancelled.
	ErrTaskCancelled = fmt.Errorf("task cancelled")
	// ErrTaskOngoing is returned when a task is still ongoing.
	ErrTaskOngoing = fmt.Errorf("task ongoing")
)

// AsyncTaskOpts contains options for asynchronous tasks.
type AsyncTaskOpts struct {
	// RequestID is the ID of the request. This will be used to set the request ID header.
	RequestID string
}

// ToRequestHeaders converts the options to a map of request headers.
func (o AsyncTaskOpts) ToRequestHeaders() map[string]interface{} {
	if o.RequestID == "" {
		return nil
	}
	headers := make(map[string]interface{}, 1)
	headers[RequestIDHeaderName] = o.RequestID
	return headers
}

// PrismClient is the interface for interacting with Prism.
type PrismClient interface {
	GetTaskData(taskID string) ([]prismcommonapi.KVPair, error)
}

// Prism returns a PrismClient.
func (c *client) Prism() PrismClient {
	return &prismClient{v4Client: c.v4Client}
}

type prismClient struct {
	v4Client *v4.Client
}

func (p *prismClient) GetTaskData(taskID string) ([]prismcommonapi.KVPair, error) {
	task, err := p.v4Client.TasksApiInstance.GetTaskById(utils.StringPtr(taskID))
	if err != nil {
		return nil, fmt.Errorf("failed to get task %s: %w", taskID, err)
	}

	taskData, ok := task.GetData().(prismapi.Task)
	if !ok {
		return nil, fmt.Errorf("unexpected task data type %[1]T: %+[1]v", task.GetData())
	}

	marshaledTaskData, err := json.Marshal(taskData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task data: %w", err)
	}

	if taskData.Status == nil {
		return nil, fmt.Errorf("%w: %s", ErrTaskOngoing, string(marshaledTaskData))
	}

	switch *taskData.Status {
	case prismapi.TASKSTATUS_SUCCEEDED:
		return taskData.CompletionDetails, nil
	case prismapi.TASKSTATUS_FAILED:
		return nil, fmt.Errorf("%w: %s", ErrTaskFailed, string(marshaledTaskData))
	case prismapi.TASKSTATUS_CANCELED:
		return nil, fmt.Errorf("%w: %s", ErrTaskCancelled, string(marshaledTaskData))
	default:
		return nil, fmt.Errorf("%w: %s", ErrTaskOngoing, string(marshaledTaskData))
	}
}
