// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	networkingprismapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/prism/v4/config"
	prismModels "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"

	convergedv4 "github.com/nutanix-cloud-native/prism-go-client/converged/v4"
)

const (
	// completionDetailsIPsKey is the name of the KVPair that Prism Central adds
	// to a subnet IP reserve/unreserve task's completion details. PC reuses the
	// same KVPair name for both operations.
	completionDetailsIPsKey = "reserved_or_unreserved_ips"

	// reservedIPsValueField is the field inside that KVPair's JSON value that
	// lists the IPs of a reserve task, e.g. {"reserved_ips": [...]}.
	reservedIPsValueField = "reserved_ips"

	// unreservedIPsValueField is the field inside that KVPair's JSON value that
	// lists the IPs of an unreserve task, e.g. {"unreserved_ips": [...]}.
	unreservedIPsValueField = "unreserved_ips"

	taskPollInterval = time.Second
)

func taskExtID(taskRef *networkingprismapi.TaskReference) (string, error) {
	if taskRef == nil || taskRef.ExtId == nil || *taskRef.ExtId == "" {
		return "", fmt.Errorf("empty task reference")
	}
	return *taskRef.ExtId, nil
}

// waitForIPTask polls Tasks.Get until the subnet IP reserve/unreserve task
// reaches a terminal state, then extracts IPs from completion details.
// When requireIPs is true, a succeeded task with no IP list is an error.
func waitForIPTask(
	ctx context.Context,
	v4Client *convergedv4.Client,
	taskID string,
	valueField string,
	requireIPs bool,
) ([]string, error) {
	if v4Client == nil || v4Client.Tasks == nil {
		return nil, fmt.Errorf("prism task client is not initialized")
	}

	ticker := time.NewTicker(taskPollInterval)
	defer ticker.Stop()

	for {
		task, err := v4Client.Tasks.Get(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get task %s: %w", taskID, err)
		}
		if task == nil || task.Status == nil {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("task wait canceled: %w", ctx.Err())
			case <-ticker.C:
				continue
			}
		}

		switch *task.Status {
		case prismModels.TASKSTATUS_SUCCEEDED:
			ips, found, err := ipsFromTask(task, valueField)
			if err != nil {
				return nil, err
			}
			if requireIPs && !found {
				return nil, fmt.Errorf("task %s completion details missing %s", taskID, valueField)
			}
			return ips, nil
		case prismModels.TASKSTATUS_FAILED,
			prismModels.TASKSTATUS_CANCELED,
			prismModels.TASKSTATUS_CANCELING:
			return nil, fmt.Errorf("task %s failed: %s", taskID, taskFailureDetail(task))
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("task wait canceled: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func taskFailureDetail(task *prismModels.Task) string {
	if task == nil {
		return "task failed"
	}

	parts := make([]string, 0, len(task.ErrorMessages)+1)
	for _, msg := range task.ErrorMessages {
		if msg.Message != nil && *msg.Message != "" {
			parts = append(parts, *msg.Message)
		}
	}
	if task.LegacyErrorMessage != nil && *task.LegacyErrorMessage != "" {
		parts = append(parts, *task.LegacyErrorMessage)
	}
	if len(parts) == 0 {
		return "task failed"
	}
	return strings.Join(parts, "; ")
}

// ipsFromTask extracts IP addresses from a subnet IP reserve/unreserve task's
// completion details. The IPs live in the KVPair named completionDetailsIPsKey,
// whose value is a JSON document keyed by valueField. The returned bool reports
// whether the KVPair carrying valueField was found.
func ipsFromTask(task *prismModels.Task, valueField string) ([]string, bool, error) {
	if task == nil {
		return nil, false, nil
	}

	for i := range task.CompletionDetails {
		detail := &task.CompletionDetails[i]
		if detail.Name == nil || *detail.Name != completionDetailsIPsKey || detail.Value == nil {
			continue
		}
		value := detail.Value.GetValue()
		if value == nil {
			continue
		}

		marshaledValue, err := json.Marshal(value)
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal IP response: %w", err)
		}
		unquotedValue, err := strconv.Unquote(string(marshaledValue))
		if err != nil {
			return nil, false, fmt.Errorf(
				"failed to unquote IP response %s: %w",
				marshaledValue,
				err,
			)
		}

		var response map[string][]string
		if err := json.Unmarshal([]byte(unquotedValue), &response); err != nil {
			return nil, false, fmt.Errorf(
				"failed to unmarshal IP response %s: %w",
				unquotedValue,
				err,
			)
		}
		ips, ok := response[valueField]
		if !ok {
			continue
		}
		return ips, true, nil
	}

	return nil, false, nil
}
