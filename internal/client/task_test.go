// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"errors"
	"testing"

	commonModels "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/common/v1/config"
	prismModels "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
	prismErrors "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/error"
	"k8s.io/utils/ptr"
)

func TestIpsFromTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		task       *prismModels.Task
		valueField string
		wantIPs    []string
		wantFound  bool
		wantErr    bool
	}{
		{
			name:       "reserved ips",
			task:       succeededIPTask(reservedIPsValueField, "10.0.0.1", "10.0.0.2"),
			valueField: reservedIPsValueField,
			wantIPs:    []string{"10.0.0.1", "10.0.0.2"},
			wantFound:  true,
		},
		{
			name:       "unreserved ips",
			task:       succeededIPTask(unreservedIPsValueField, "10.0.0.3"),
			valueField: unreservedIPsValueField,
			wantIPs:    []string{"10.0.0.3"},
			wantFound:  true,
		},
		{
			name:       "missing reserved list",
			task:       succeededIPTask(unreservedIPsValueField, "10.0.0.3"),
			valueField: reservedIPsValueField,
			wantFound:  false,
		},
		{
			name:       "empty completion details",
			task:       &prismModels.Task{},
			valueField: reservedIPsValueField,
			wantFound:  false,
		},
		{
			name:       "nil task",
			task:       nil,
			valueField: reservedIPsValueField,
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ips, found, err := ipsFromTask(tt.task, tt.valueField)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ipsFromTask() unexpected error: %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if len(ips) != len(tt.wantIPs) {
				t.Fatalf("ips = %v, want %v", ips, tt.wantIPs)
			}
			for i := range tt.wantIPs {
				if ips[i] != tt.wantIPs[i] {
					t.Fatalf("ips = %v, want %v", ips, tt.wantIPs)
				}
			}
		})
	}
}

func TestUnreservationAlreadyReleased(t *testing.T) {
	t.Parallel()

	if !unreservationAlreadyReleased(errors.New("No IP addresses exist with context: abc")) {
		t.Fatal("expected already-released message to match")
	}
	if unreservationAlreadyReleased(errors.New("subnet not found")) {
		t.Fatal("expected unrelated error not to match")
	}
	if !unreservationAlreadyReleased(
		errors.New("task xyz failed: No IP addresses exist with context: abc"),
	) {
		t.Fatal("expected task failure wrapping already-released message to match")
	}
}

func TestTaskFailureDetail(t *testing.T) {
	t.Parallel()

	task := &prismModels.Task{
		ErrorMessages: []prismErrors.AppMessage{
			{Message: ptr.To("No IP addresses exist with context: claim-uid")},
		},
		LegacyErrorMessage: ptr.To("legacy detail"),
	}
	got := taskFailureDetail(task)
	want := "No IP addresses exist with context: claim-uid; legacy detail"
	if got != want {
		t.Fatalf("taskFailureDetail() = %q, want %q", got, want)
	}
}

func succeededIPTask(valueField string, ips ...string) *prismModels.Task {
	status := prismModels.TASKSTATUS_SUCCEEDED
	kv := commonModels.NewKVPair()
	kv.Name = ptr.To(completionDetailsIPsKey)
	value := commonModels.NewOneOfKVPairValue()

	payloadBytes, err := json.Marshal(map[string][]string{valueField: ips})
	if err != nil {
		panic(err)
	}
	if err := value.SetValue(string(payloadBytes)); err != nil {
		panic(err)
	}
	kv.Value = value

	return &prismModels.Task{
		Status:            &status,
		CompletionDetails: []commonModels.KVPair{*kv},
	}
}
