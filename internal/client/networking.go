// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/google/uuid"
	networkingapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	networkingprismapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/prism/v4/config"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"

	"github.com/nutanix-cloud-native/prism-go-client/utils"
)

type ipReserveSpec struct {
	networkingapi.IpReserveSpec
}

type IPReservationTypeFunc func(*ipReserveSpec)

func ReserveIPCount(count int64) IPReservationTypeFunc {
	return func(spec *ipReserveSpec) {
		spec.ReserveType = ptr.To(networkingapi.RESERVETYPE_IP_ADDRESS_COUNT)
		spec.Count = &count
	}
}

type ReserveIPOpts struct {
	AsyncTaskOpts

	Cluster       string
	ClientContext string
}

type NetworkingClient interface {
	ReserveIP(reserveType IPReservationTypeFunc, subnet string, opts ReserveIPOpts) (net.IP, error)
	UnreserveIP(unreserveType IPUnreservationTypeFunc, subnet string, opts UnreserveIPOpts) error
	GetSubnet(subnet string, opts GetSubnetOpts) (*Subnet, error)
}

func (c *client) Networking() NetworkingClient {
	return &networkingClient{
		client: c,
	}
}

type networkingClient struct {
	*client
}

func (n *networkingClient) ReserveIP(
	reserveType IPReservationTypeFunc, subnet string, opts ReserveIPOpts,
) (ip net.IP, err error) {
	apiSubnet, err := n.GetSubnet(subnet, GetSubnetOpts{Cluster: opts.Cluster})
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet %s: %w", subnet, err)
	}

	subnetUUID := apiSubnet.ExtID()

	ipReserveSpec := ipReserveSpec{}
	reserveType(&ipReserveSpec)

	if opts.ClientContext != "" {
		ipReserveSpec.ClientContext = ptr.To(opts.ClientContext)
	}

	reserveIPResponse, err := n.v4Client.SubnetIPReservationApi.ReserveIpsBySubnetId(
		utils.StringPtr(subnetUUID.String()),
		&ipReserveSpec.IpReserveSpec,
		opts.AsyncTaskOpts.ToRequestHeaders(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve IP in subnet %s: %w", subnet, err)
	}

	responseData, ok := reserveIPResponse.GetData().(networkingprismapi.TaskReference)
	if !ok {
		return nil, fmt.Errorf(
			"unexpected response data type %[1]T: %+[1]v",
			reserveIPResponse.GetData(),
		)
	}
	if responseData.ExtId == nil {
		return nil, fmt.Errorf(
			"no task id found in response: %+[1]v",
			reserveIPResponse.GetData(),
		)
	}

	result, err := n.client.Prism().GetTaskData(
		*responseData.ExtId,
	)
	if err != nil {
		return nil, fmt.Errorf("task has not successfully completed: %w", err)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no IP address reserved")
	}
	if len(result) > 1 {
		return nil, fmt.Errorf("unexpected multiple results returned: %+v", result)
	}

	marshaledResponseBytes, _ := json.Marshal(result[0].Value)
	marshaledResponse, err := strconv.Unquote(string(marshaledResponseBytes))
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unquote reserved IP response %s: %w",
			marshaledResponseBytes,
			err,
		)
	}

	type reservedIPs struct {
		ReservedIPs []string `json:"reserved_ips"`
	}

	var response reservedIPs
	if err := json.Unmarshal([]byte(marshaledResponse), &response); err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal reserved IP response %s: %w",
			marshaledResponse,
			err,
		)
	}

	if len(response.ReservedIPs) == 0 {
		return nil, fmt.Errorf("no IP address reserved")
	}
	if len(response.ReservedIPs) > 1 {
		return nil, fmt.Errorf("unexpected multiple IPs reserved: %+v", response.ReservedIPs)
	}

	reservedIP := net.ParseIP(response.ReservedIPs[0])
	if reservedIP == nil {
		return nil, fmt.Errorf("failed to parse reserved IP %q", response.ReservedIPs[0])
	}

	return reservedIP, nil
}

type ipUnreserveSpec struct {
	networkingapi.IpUnreserveSpec
}

type IPUnreservationTypeFunc func(*ipUnreserveSpec)

func UnreserveIPClientContext(clientContext string) IPUnreservationTypeFunc {
	return func(spec *ipUnreserveSpec) {
		spec.UnreserveType = ptr.To(networkingapi.UNRESERVETYPE_CONTEXT)
		spec.ClientContext = ptr.To(clientContext)
	}
}

type UnreserveIPOpts struct {
	AsyncTaskOpts

	Cluster string
}

func (n *networkingClient) UnreserveIP(
	unreserveType IPUnreservationTypeFunc, subnet string, opts UnreserveIPOpts,
) error {
	apiSubnet, err := n.GetSubnet(subnet, GetSubnetOpts{Cluster: opts.Cluster})
	if err != nil {
		return fmt.Errorf("failed to get subnet %s: %w", subnet, err)
	}

	subnetUUID := apiSubnet.ExtID()

	ipUnreserveSpec := ipUnreserveSpec{}
	unreserveType(&ipUnreserveSpec)

	unreserveIPResponse, err := n.v4Client.SubnetIPReservationApi.UnreserveIpsBySubnetId(
		utils.StringPtr(subnetUUID.String()),
		&ipUnreserveSpec.IpUnreserveSpec,
		opts.AsyncTaskOpts.ToRequestHeaders(),
	)
	if err != nil {
		return fmt.Errorf("failed to unreserve IP in subnet %s: %w", subnet, err)
	}

	responseData, ok := unreserveIPResponse.GetData().(networkingprismapi.TaskReference)
	if !ok {
		return fmt.Errorf(
			"unexpected response data type %[1]T: %+[1]v",
			unreserveIPResponse.GetData(),
		)
	}
	if responseData.ExtId == nil {
		return fmt.Errorf("no task id found in response: %+v", unreserveIPResponse.GetData())
	}

	_, err = n.client.Prism().GetTaskData(*responseData.ExtId)
	if err != nil {
		return fmt.Errorf("task has not successfully completed: %w", err)
	}

	return nil
}

type Subnet struct {
	extID uuid.UUID
}

func (s *Subnet) ExtID() uuid.UUID {
	return s.extID
}

type GetSubnetOpts struct {
	Cluster string
}

func (n *networkingClient) GetSubnet(
	subnetExtIDOrName string,
	opts GetSubnetOpts,
) (*Subnet, error) {
	var errs []error

	subnetUUID, err := uuid.Parse(subnetExtIDOrName)
	if err == nil {
		subnet, errByExtID := n.getSubnetByExtID(subnetUUID)
		if errByExtID == nil {
			return subnet, nil
		}
		errs = append(errs, errByExtID)
	}

	subnet, errByName := n.getSubnetByName(subnetExtIDOrName, opts)
	if errByName != nil {
		errs = append(errs, errByName)
		aggErr := kerrors.NewAggregate(errs)
		return nil, fmt.Errorf("failed to get subnet %q: %w", subnetExtIDOrName, aggErr)
	}

	return subnet, nil
}

func (n *networkingClient) getSubnetByName(subnetName string, opts GetSubnetOpts) (*Subnet, error) {
	filter := fmt.Sprintf(`name eq '%s'`, subnetName)
	if opts.Cluster != "" {
		apiCluster, err := n.client.Cluster().GetCluster(opts.Cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster %s: %w", opts.Cluster, err)
		}

		filter += fmt.Sprintf(` and clusterReference eq '%s'`, apiCluster.ExtID())
	}

	response, err := n.v4Client.SubnetsApiInstance.ListSubnets(
		nil,
		nil,
		utils.StringPtr(filter),
		nil,
		nil,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find subnet uuid for subnet %q: %w",
			subnetName,
			err,
		)
	}
	subnets := response.GetData()
	if subnets == nil {
		return nil, fmt.Errorf("no subnet found with name %q", subnetName)
	}

	switch apiSubnets := subnets.(type) {
	case []networkingapi.Subnet:
		if len(apiSubnets) == 0 {
			return nil, fmt.Errorf("no subnet found with name %q", subnetName)
		}
		if len(apiSubnets) > 1 {
			return nil, fmt.Errorf("multiple subnets (%d) found with name %q", len(apiSubnets), subnetName)
		}

		extID := *apiSubnets[0].ExtId
		subnetUUID, err := uuid.Parse(extID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet uuid %q for cluster %q: %w", extID, opts.Cluster, err)
		}

		return &Subnet{
			extID: subnetUUID,
		}, nil
	case []networkingapi.SubnetProjection:
		if len(apiSubnets) == 0 {
			return nil, fmt.Errorf("no subnet found with name %s", subnetName)
		}
		if len(apiSubnets) > 1 {
			return nil, fmt.Errorf("multiple subnets (%d) found with name %q", len(apiSubnets), subnetName)
		}

		extID := *apiSubnets[0].ExtId
		subnetUUID, err := uuid.Parse(extID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet uuid %q for cluster %q: %w", extID, opts.Cluster, err)
		}

		return &Subnet{
			extID: subnetUUID,
		}, nil
	default:
		return nil, fmt.Errorf("unknown response: %+v", subnets)
	}
}

func (n *networkingClient) getSubnetByExtID(subnetExtID uuid.UUID) (*Subnet, error) {
	response, err := n.v4Client.SubnetsApiInstance.GetSubnetById(
		utils.StringPtr(subnetExtID.String()),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find subnet with extID %q: %w",
			subnetExtID,
			err,
		)
	}
	subnet := response.GetData()
	if subnet == nil {
		return nil, fmt.Errorf("no subnet found with extID %q", subnetExtID)
	}

	switch apiSubnet := subnet.(type) {
	case *networkingapi.Subnet:
		if apiSubnet.ExtId == nil {
			return nil, fmt.Errorf("no extID found for subnet %q", subnetExtID)
		}
		subnetUUID, err := uuid.Parse(*apiSubnet.ExtId)
		if err != nil {
			return nil,
				fmt.Errorf(
					"failed to parse subnet extID %q for subnet %q: %w",
					*apiSubnet.ExtId,
					subnetExtID,
					err,
				)
		}

		return &Subnet{
			extID: subnetUUID,
		}, nil
	default:
		return nil, fmt.Errorf("unknown response: %+v", subnet)
	}
}
