// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"k8s.io/utils/ptr"

	"github.com/nutanix-cloud-native/prism-go-client/converged"
	convergedv4 "github.com/nutanix-cloud-native/prism-go-client/converged/v4"

	commonapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/common/v1/config"
	networkingapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	prismModels "github.com/nutanix/ntnx-api-golang-clients/prism-go-client/v4/models/prism/v4/config"
)

// FloatingIPAssociationType identifies how a floating IP is associated.
type FloatingIPAssociationType string

const (
	FloatingIPAssociationNone      FloatingIPAssociationType = "notAssociated"
	FloatingIPAssociationPrivateIP FloatingIPAssociationType = "privateIP"
	FloatingIPAssociationVMNic     FloatingIPAssociationType = "vmNic"
)

// FloatingIP is a provider-facing view of a Prism floating IP.
type FloatingIP struct {
	ExtID               string
	Name                string
	Address             string
	ExternalSubnetExtID string
	AssociationType     FloatingIPAssociationType
	PrivateIPAddress    string
	PrivateIPVPCExtID   string
	VMNicExtID          string
}

// CreateFloatingIPOpts configures floating IP creation.
type CreateFloatingIPOpts struct {
	Name                string
	ExternalSubnetExtID string
	// ProjectExtID is a Prism project UUID or name. Names are resolved before create.
	ProjectExtID string
	Address      string
}

// PrivateIPAssociation associates a floating IP with a private IP in a VPC.
type PrivateIPAssociation struct {
	Address  string
	VPCExtID string
}

// VMNicAssociation associates a floating IP with a VM NIC.
type VMNicAssociation struct {
	VMNicExtID string
}

// FloatingIPsClient interacts with the Prism Floating IPs API.
type FloatingIPsClient interface {
	Get(ctx context.Context, extID string) (*FloatingIP, error)
	ListByName(ctx context.Context, name string) ([]FloatingIP, error)
	CreateAsync(ctx context.Context, opts CreateFloatingIPOpts) (taskID string, err error)
	WaitForCreate(ctx context.Context, taskID string) (*FloatingIP, error)
	Delete(ctx context.Context, extID string) error
	AssignPrivateIP(
		ctx context.Context,
		extID string,
		association PrivateIPAssociation,
	) (*FloatingIP, error)
	AssignVMNic(
		ctx context.Context,
		extID string,
		association VMNicAssociation,
	) (*FloatingIP, error)
	Deassign(ctx context.Context, extID string) (*FloatingIP, error)
}

func (c *client) FloatingIPs() FloatingIPsClient {
	return &floatingIPsClient{client: c}
}

type floatingIPsClient struct {
	*client
}

func (f *floatingIPsClient) Get(ctx context.Context, extID string) (*FloatingIP, error) {
	apiFIP, err := f.v4Client.FloatingIps.Get(ctx, extID)
	if err != nil {
		return nil, fmt.Errorf("failed to get floating IP %q: %w", extID, err)
	}
	return floatingIPFromAPI(apiFIP)
}

func (f *floatingIPsClient) ListByName(ctx context.Context, name string) ([]FloatingIP, error) {
	filter := fmt.Sprintf(`name eq '%s'`, strings.ReplaceAll(name, "'", "''"))
	apiFIPs, err := f.v4Client.FloatingIps.List(ctx, converged.WithFilter(filter))
	if err != nil {
		return nil, fmt.Errorf("failed to list floating IPs named %q: %w", name, err)
	}

	out := make([]FloatingIP, 0, len(apiFIPs))
	for i := range apiFIPs {
		fip, err := floatingIPFromAPI(&apiFIPs[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *fip)
	}
	return out, nil
}

func (f *floatingIPsClient) CreateAsync(
	ctx context.Context,
	opts CreateFloatingIPOpts,
) (string, error) {
	body := networkingapi.NewFloatingIp()
	body.Name = ptr.To(opts.Name)
	body.ExternalSubnetReference = ptr.To(opts.ExternalSubnetExtID)
	if opts.ProjectExtID != "" {
		projectExtID, err := f.resolveProjectExtID(ctx, opts.ProjectExtID)
		if err != nil {
			return "", err
		}
		body.ProjectExtId = ptr.To(projectExtID)
	}
	if opts.Address != "" {
		addr := networkingapi.NewFloatingIPAddress()
		addr.Ipv4 = networkingapi.NewFloatingIPv4Address()
		addr.Ipv4.Value = ptr.To(opts.Address)
		addr.Ipv4.PrefixLength = nil
		body.FloatingIp = addr
	}

	op, err := f.v4Client.FloatingIps.CreateAsync(ctx, body)
	if err != nil {
		return "", fmt.Errorf("failed to create floating IP %q: %w", opts.Name, err)
	}
	taskID := op.UUID()
	if taskID == "" {
		return "", fmt.Errorf("create floating IP %q returned empty task id", opts.Name)
	}
	return taskID, nil
}

// resolveProjectExtID accepts a project UUID or name and returns the project UUID.
func (f *floatingIPsClient) resolveProjectExtID(
	ctx context.Context,
	projectNameOrID string,
) (string, error) {
	if _, err := uuid.Parse(projectNameOrID); err == nil {
		return projectNameOrID, nil
	}
	if f.v4Client == nil || f.v4Client.Projects == nil {
		return "", fmt.Errorf("prism projects client is not initialized")
	}

	filter := fmt.Sprintf(`name eq '%s'`, strings.ReplaceAll(projectNameOrID, "'", "''"))
	projects, err := f.v4Client.Projects.List(ctx, converged.WithFilter(filter))
	if err != nil {
		return "", fmt.Errorf("failed to resolve project %q: %w", projectNameOrID, err)
	}
	if len(projects) == 0 {
		return "", fmt.Errorf("no project found with name %q", projectNameOrID)
	}
	if len(projects) > 1 {
		return "", fmt.Errorf("multiple projects (%d) found with name %q", len(projects), projectNameOrID)
	}
	if projects[0].ExtId == nil || *projects[0].ExtId == "" {
		return "", fmt.Errorf("project %q has no extID", projectNameOrID)
	}
	return *projects[0].ExtId, nil
}

func (f *floatingIPsClient) WaitForCreate(ctx context.Context, taskID string) (*FloatingIP, error) {
	if f.v4Client == nil || f.v4Client.Tasks == nil {
		return nil, fmt.Errorf("prism task client is not initialized")
	}

	ticker := time.NewTicker(taskPollInterval)
	defer ticker.Stop()

	for {
		task, err := f.v4Client.Tasks.Get(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to get create task %s: %w", taskID, err)
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
			extID, err := floatingIPExtIDFromTask(task)
			if err != nil {
				return nil, err
			}
			return f.Get(ctx, extID)
		case prismModels.TASKSTATUS_FAILED,
			prismModels.TASKSTATUS_CANCELED,
			prismModels.TASKSTATUS_CANCELING:
			return nil, fmt.Errorf(
				"create floating IP task %s failed: %s",
				taskID,
				taskFailureDetail(task),
			)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("task wait canceled: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (f *floatingIPsClient) Delete(ctx context.Context, extID string) error {
	if err := f.v4Client.FloatingIps.Delete(ctx, extID); err != nil {
		return fmt.Errorf("failed to delete floating IP %q: %w", extID, err)
	}
	return nil
}

func (f *floatingIPsClient) AssignPrivateIP(
	ctx context.Context,
	extID string,
	association PrivateIPAssociation,
) (*FloatingIP, error) {
	addr, err := netip.ParseAddr(association.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private IP %q: %w", association.Address, err)
	}
	if !addr.Is4() {
		return nil, fmt.Errorf("private IP %q must be IPv4", association.Address)
	}

	privateIP := commonapi.NewIPAddress()
	privateIP.Ipv4 = commonapi.NewIPv4Address()
	privateIP.Ipv4.Value = ptr.To(addr.String())

	assoc := networkingapi.NewPrivateIpAssociation()
	assoc.PrivateIp = privateIP
	assoc.VpcReference = ptr.To(association.VPCExtID)

	apiFIP, err := f.v4Client.FloatingIps.AssignFloatingIp(ctx, extID, assoc)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to assign floating IP %q to private IP %q: %w",
			extID,
			association.Address,
			err,
		)
	}
	return floatingIPFromAPI(apiFIP)
}

func (f *floatingIPsClient) AssignVMNic(
	ctx context.Context,
	extID string,
	association VMNicAssociation,
) (*FloatingIP, error) {
	assoc := networkingapi.NewVmNicAssociation()
	assoc.VmNicReference = ptr.To(association.VMNicExtID)

	apiFIP, err := f.v4Client.FloatingIps.AssignFloatingIp(ctx, extID, assoc)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to assign floating IP %q to VM NIC %q: %w",
			extID,
			association.VMNicExtID,
			err,
		)
	}
	return floatingIPFromAPI(apiFIP)
}

func (f *floatingIPsClient) Deassign(ctx context.Context, extID string) (*FloatingIP, error) {
	apiFIP, err := f.v4Client.FloatingIps.DeassignFloatingIp(ctx, extID)
	if err != nil {
		return nil, fmt.Errorf("failed to deassign floating IP %q: %w", extID, err)
	}
	return floatingIPFromAPI(apiFIP)
}

func floatingIPExtIDFromTask(task *prismModels.Task) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task is nil")
	}
	for _, entity := range task.EntitiesAffected {
		if entity.ExtId == nil || *entity.ExtId == "" {
			continue
		}
		return *entity.ExtId, nil
	}
	return "", fmt.Errorf(
		"task %s succeeded without a floating IP entity",
		ptr.Deref(task.ExtId, ""),
	)
}

func floatingIPFromAPI(apiFIP *networkingapi.FloatingIp) (*FloatingIP, error) {
	if apiFIP == nil {
		return nil, fmt.Errorf("floating IP is nil")
	}
	if apiFIP.ExtId == nil || *apiFIP.ExtId == "" {
		return nil, fmt.Errorf("floating IP has no extID")
	}

	out := &FloatingIP{
		ExtID:               *apiFIP.ExtId,
		AssociationType:     FloatingIPAssociationNone,
		ExternalSubnetExtID: ptr.Deref(apiFIP.ExternalSubnetReference, ""),
		Name:                ptr.Deref(apiFIP.Name, ""),
	}

	if apiFIP.FloatingIpValue != nil && *apiFIP.FloatingIpValue != "" {
		out.Address = *apiFIP.FloatingIpValue
	} else if apiFIP.FloatingIp != nil && apiFIP.FloatingIp.Ipv4 != nil && apiFIP.FloatingIp.Ipv4.Value != nil {
		out.Address = *apiFIP.FloatingIp.Ipv4.Value
	}

	if apiFIP.AssociationStatus != nil {
		switch *apiFIP.AssociationStatus {
		case networkingapi.ASSOCIATIONSTATUS_ASSOCIATED_TO_PRIVATE_IP:
			out.AssociationType = FloatingIPAssociationPrivateIP
		case networkingapi.ASSOCIATIONSTATUS_ASSOCIATED_TO_VNIC:
			out.AssociationType = FloatingIPAssociationVMNic
		default:
			out.AssociationType = FloatingIPAssociationNone
		}
	}

	if assoc := apiFIP.GetAssociation(); assoc != nil {
		switch v := assoc.(type) {
		case networkingapi.PrivateIpAssociation:
			fillPrivateIPAssociation(out, &v)
		case *networkingapi.PrivateIpAssociation:
			fillPrivateIPAssociation(out, v)
		case networkingapi.VmNicAssociation:
			fillVMNicAssociation(out, &v)
		case *networkingapi.VmNicAssociation:
			fillVMNicAssociation(out, v)
		}
	}

	if out.PrivateIPAddress == "" && apiFIP.PrivateIp != nil {
		out.PrivateIPAddress = *apiFIP.PrivateIp
		if out.AssociationType == FloatingIPAssociationNone {
			out.AssociationType = FloatingIPAssociationPrivateIP
		}
	}
	if out.PrivateIPVPCExtID == "" && apiFIP.VpcReference != nil {
		out.PrivateIPVPCExtID = *apiFIP.VpcReference
	}
	if out.VMNicExtID == "" && apiFIP.VmNicReference != nil {
		out.VMNicExtID = *apiFIP.VmNicReference
		if out.AssociationType == FloatingIPAssociationNone {
			out.AssociationType = FloatingIPAssociationVMNic
		}
	}

	return out, nil
}

func fillPrivateIPAssociation(out *FloatingIP, assoc *networkingapi.PrivateIpAssociation) {
	if assoc == nil {
		return
	}
	out.AssociationType = FloatingIPAssociationPrivateIP
	if assoc.VpcReference != nil {
		out.PrivateIPVPCExtID = *assoc.VpcReference
	}
	if assoc.PrivateIp != nil && assoc.PrivateIp.Ipv4 != nil && assoc.PrivateIp.Ipv4.Value != nil {
		out.PrivateIPAddress = *assoc.PrivateIp.Ipv4.Value
	}
}

func fillVMNicAssociation(out *FloatingIP, assoc *networkingapi.VmNicAssociation) {
	if assoc == nil {
		return
	}
	out.AssociationType = FloatingIPAssociationVMNic
	if assoc.VmNicReference != nil {
		out.VMNicExtID = *assoc.VmNicReference
	}
}

// Ensure FloatingIpsService is present on the converged client used by this package.
var _ = (*convergedv4.Client)(nil)
