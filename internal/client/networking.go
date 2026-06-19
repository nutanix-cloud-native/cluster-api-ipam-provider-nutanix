// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	commonapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/common/v1/config"
	networkingapi "github.com/nutanix/ntnx-api-golang-clients/networking-go-client/v4/models/networking/v4/config"
	"go4.org/netipx"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"

	"github.com/nutanix-cloud-native/prism-go-client/converged"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/poolutil"
)

// internalReserveSpec holds the configuration for reserving an IP address. This is an unexported type to
// prevent users from mutating the IP reservation configuration without going through the supported
// IPReservationTypeFunc functions.
type internalReserveSpec struct {
	networkingapi.IpReserveSpec
}

// IPReservationTypeFunc is a function that configures an IP reservation.
type IPReservationTypeFunc func(*internalReserveSpec)

// ReserveIPCountFunc configures the IP reservation to reserve a specific number of IP addresses.
func ReserveIPCountFunc(count int64) IPReservationTypeFunc {
	return func(spec *internalReserveSpec) {
		spec.ReserveType = ptr.To(networkingapi.RESERVETYPE_IP_ADDRESS_COUNT)
		spec.Count = &count
	}
}

// ReserveIPRangeFunc configures the IP reservation to reserve a range of IP addresses.
// A range out of two IPs separated by a hyphen.
func ReserveIPRangeFunc(ipRange string) (IPReservationTypeFunc, error) {
	ipxRange, err := netipx.ParseIPRange(ipRange)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP range %s: %w", ipRange, err)
	}

	builder := &netipx.IPSetBuilder{}
	builder.AddRange(ipxRange)
	ipSet, err := builder.IPSet()
	if err != nil {
		return nil, fmt.Errorf("failed to create IP set from range %s: %w", ipRange, err)
	}

	ipCount, err := poolutil.IPSetCount(ipSet)
	if err != nil {
		return nil, fmt.Errorf("failed to count IP set: %w", err)
	}

	startIPAddress := commonapi.NewIPAddress()
	startAddr := ipxRange.From()
	switch {
	case startAddr.Is4():
		ipv4 := commonapi.NewIPv4Address()
		ipv4.Value = ptr.To(startAddr.String())
		startIPAddress.Ipv4 = ipv4
	case startAddr.Is6():
		ipv6 := commonapi.NewIPv6Address()
		ipv6.Value = ptr.To(startAddr.String())
		startIPAddress.Ipv6 = ipv6
	default:
		return nil, fmt.Errorf("unexpected IP address type: %s", startAddr)
	}

	return func(spec *internalReserveSpec) {
		spec.ReserveType = ptr.To(networkingapi.RESERVETYPE_IP_ADDRESS_RANGE)
		spec.Count = ptr.To(ipCount)
		spec.StartIpAddress = startIPAddress
	}, nil
}

// ReserveIPListFunc configures the IP reservation to reserve a list of specific IP addresses.
func ReserveIPListFunc(ips ...string) (IPReservationTypeFunc, error) {
	ipAddrs := make([]commonapi.IPAddress, 0, len(ips))

	for _, ip := range ips {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, fmt.Errorf("failed to parse IP address %q: %w", ip, err)
		}

		ipAddr := commonapi.NewIPAddress()
		switch {
		case addr.Is4():
			ipv4 := commonapi.NewIPv4Address()
			ipv4.Value = ptr.To(addr.String())
			ipAddr.Ipv4 = ipv4
		case addr.Is6():
			ipv6 := commonapi.NewIPv6Address()
			ipv6.Value = ptr.To(addr.String())
			ipAddr.Ipv6 = ipv6
		default:
			return nil, fmt.Errorf("unexpected IP address type: %s", addr)
		}

		ipAddrs = append(ipAddrs, *ipAddr)
	}

	return func(spec *internalReserveSpec) {
		spec.ReserveType = ptr.To(networkingapi.RESERVETYPE_IP_ADDRESS_LIST)
		spec.IpAddresses = ipAddrs
	}, nil
}

// ReserveIPOpts holds optional configuration for reserving an IP address.
type ReserveIPOpts struct {
	// Cluster is the name of the cluster where the subnet is located. Only required if using the subnet
	// name rather than the extIDi.
	Cluster string

	// ClientContext is an optional context to associate with the reservation. This can be used to unreserve
	// the IP address later, ensuring that no IPs are leaked.
	ClientContext string
}

// NetworkingClient is the interface for interacting with the networking API.
type NetworkingClient interface {
	ReserveIPs(
		ctx context.Context,
		reserveType IPReservationTypeFunc,
		subnet string,
		opts ReserveIPOpts,
	) ([]netip.Addr, error)
	UnreserveIPs(
		ctx context.Context,
		unreserveType IPUnreservationTypeFunc,
		subnet string,
		opts UnreserveIPOpts,
	) error
	GetSubnet(ctx context.Context, subnet string, opts GetSubnetOpts) (*Subnet, error)
}

// Networking returns a client for interacting with the networking API.
func (c *client) Networking() NetworkingClient {
	return &networkingClient{
		client: c,
	}
}

// networkingClient is the implementation of the NetworkingClient interface.
type networkingClient struct {
	*client
}

func (n *networkingClient) ReserveIPs(
	ctx context.Context, reserveType IPReservationTypeFunc, subnet string, opts ReserveIPOpts,
) ([]netip.Addr, error) {
	apiSubnet, err := n.GetSubnet(ctx, subnet, GetSubnetOpts{Cluster: opts.Cluster})
	if err != nil {
		return nil, fmt.Errorf("failed to get subnet %s: %w", subnet, err)
	}

	reservation := internalReserveSpec{}
	reserveType(&reservation)

	if opts.ClientContext != "" {
		reservation.ClientContext = ptr.To(opts.ClientContext)
	}

	// ReserveIpsBySubnetId submits the reservation task and blocks until it
	// completes, returning the reserved IP addresses from the task completion
	// details.
	reservedIPs, err := n.v4Client.Subnets.ReserveIpsBySubnetId(
		ctx,
		apiSubnet.ExtID().String(),
		&reservation.IpReserveSpec,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve IP in subnet %s: %w", subnet, err)
	}

	if len(reservedIPs) == 0 {
		return nil, fmt.Errorf("no IP address reserved")
	}

	ips := make([]netip.Addr, 0, len(reservedIPs))
	for _, ip := range reservedIPs {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, fmt.Errorf("failed to parse reserved IP: %w", err)
		}
		ips = append(ips, addr)
	}

	return ips, nil
}

// internalUnreserveSpec holds the configuration for unreserving an IP address. This is an unexported type to
// prevent users from mutating the IP unreservation configuration without going through the supported
// IPUnreservationTypeFunc functions.
type internalUnreserveSpec struct {
	networkingapi.IpUnreserveSpec
}

// IPUnreservationTypeFunc is a function that configures an IP unreservation.
type IPUnreservationTypeFunc func(*internalUnreserveSpec)

// UnreserveIPClientContext configures the IP unreservation to unreserve an IP address by client context.
func UnreserveIPClientContext(clientContext string) IPUnreservationTypeFunc {
	return func(spec *internalUnreserveSpec) {
		spec.UnreserveType = ptr.To(networkingapi.UNRESERVETYPE_CONTEXT)
		spec.ClientContext = ptr.To(clientContext)
	}
}

func UnreserveIPListFunc(ips ...string) (IPUnreservationTypeFunc, error) {
	ipAddrs := make([]commonapi.IPAddress, 0, len(ips))

	for _, ip := range ips {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil, fmt.Errorf("failed to parse IP address %q: %w", ip, err)
		}

		ipAddr := commonapi.NewIPAddress()
		switch {
		case addr.Is4():
			ipv4 := commonapi.NewIPv4Address()
			ipv4.Value = ptr.To(addr.String())
			ipAddr.Ipv4 = ipv4
		case addr.Is6():
			ipv6 := commonapi.NewIPv6Address()
			ipv6.Value = ptr.To(addr.String())
			ipAddr.Ipv6 = ipv6
		default:
			return nil, fmt.Errorf("unexpected IP address type: %s", addr)
		}

		ipAddrs = append(ipAddrs, *ipAddr)
	}

	return func(spec *internalUnreserveSpec) {
		spec.UnreserveType = ptr.To(networkingapi.UNRESERVETYPE_IP_ADDRESS_LIST)
		spec.IpAddresses = ipAddrs
	}, nil
}

// UnreserveIPRangeFunc configures the IP unreservation to unreserve a range of IP addresses.
// A range out of two IPs separated by a hyphen.
func UnreserveIPRangeFunc(ipRange string) (IPUnreservationTypeFunc, error) {
	ipxRange, err := netipx.ParseIPRange(ipRange)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IP range %s: %w", ipRange, err)
	}

	builder := &netipx.IPSetBuilder{}
	builder.AddRange(ipxRange)
	ipSet, err := builder.IPSet()
	if err != nil {
		return nil, fmt.Errorf("failed to create IP set from range %s: %w", ipRange, err)
	}

	ipCount, err := poolutil.IPSetCount(ipSet)
	if err != nil {
		return nil, fmt.Errorf("failed to count IP set: %w", err)
	}

	startIPAddress := commonapi.NewIPAddress()
	startAddr := ipxRange.From()
	switch {
	case startAddr.Is4():
		ipv4 := commonapi.NewIPv4Address()
		ipv4.Value = ptr.To(startAddr.String())
		startIPAddress.Ipv4 = ipv4
	case startAddr.Is6():
		ipv6 := commonapi.NewIPv6Address()
		ipv6.Value = ptr.To(startAddr.String())
		startIPAddress.Ipv6 = ipv6
	default:
		return nil, fmt.Errorf("unexpected IP address type: %s", startAddr)
	}

	return func(spec *internalUnreserveSpec) {
		spec.UnreserveType = ptr.To(networkingapi.UNRESERVETYPE_IP_ADDRESS_RANGE)
		spec.Count = ptr.To(ipCount)
		spec.StartIpAddress = startIPAddress
	}, nil
}

// UnreserveIPOpts holds optional configuration for unreserving an IP address.
type UnreserveIPOpts struct {
	// Cluster is the name of the cluster where the subnet is located. Only required if using the subnet
	// name rather than the extID.
	Cluster string
}

func (n *networkingClient) UnreserveIPs(
	ctx context.Context, unreserveType IPUnreservationTypeFunc, subnet string, opts UnreserveIPOpts,
) error {
	apiSubnet, err := n.GetSubnet(ctx, subnet, GetSubnetOpts{Cluster: opts.Cluster})
	if err != nil {
		return fmt.Errorf("failed to get subnet %s: %w", subnet, err)
	}

	unreservation := internalUnreserveSpec{}
	unreserveType(&unreservation)

	// UnreserveIpsBySubnetId submits the unreservation task and blocks until it
	// completes.
	if err := n.v4Client.Subnets.UnreserveIpsBySubnetId(
		ctx,
		apiSubnet.ExtID().String(),
		&unreservation.IpUnreserveSpec,
	); err != nil {
		// Unreserving an IP that was never reserved (or already released) is
		// not an error from our perspective; the desired end state is reached.
		if unreservationAlreadyReleased(err) {
			return nil
		}
		return fmt.Errorf("failed to unreserve IP in subnet %s: %w", subnet, err)
	}

	return nil
}

func unreservationAlreadyReleased(err error) bool {
	return strings.Contains(err.Error(), "No IP addresses exist with context:")
}

// Subnet represents a subnet in the networking API.
type Subnet struct {
	extID  uuid.UUID
	prefix int32
}

func NewSubnet(extID uuid.UUID, prefix int32) *Subnet {
	return &Subnet{
		extID:  extID,
		prefix: prefix,
	}
}

// ExtID returns the external ID of the subnet.
func (s *Subnet) ExtID() uuid.UUID {
	return s.extID
}

// Prefix returns the subnet prefix length.
func (s *Subnet) Prefix() int32 {
	return s.prefix
}

// GetSubnetOpts holds optional configuration for getting a subnet.
type GetSubnetOpts struct {
	// Cluster is the name of the cluster where the subnet is located. Only required if using the subnet
	// name rather than the extID.
	Cluster string
}

func (n *networkingClient) GetSubnet(
	ctx context.Context,
	subnetExtIDOrName string,
	opts GetSubnetOpts,
) (*Subnet, error) {
	var errs []error

	subnetUUID, err := uuid.Parse(subnetExtIDOrName)
	if err == nil {
		subnet, errByExtID := n.getSubnetByExtID(ctx, subnetUUID)
		if errByExtID == nil {
			return subnet, nil
		}
		errs = append(errs, errByExtID)
	}

	subnet, errByName := n.getSubnetByName(ctx, subnetExtIDOrName, opts)
	if errByName != nil {
		errs = append(errs, errByName)
		aggErr := kerrors.NewAggregate(errs)
		return nil, fmt.Errorf("failed to get subnet %q: %w", subnetExtIDOrName, aggErr)
	}

	return subnet, nil
}

func (n *networkingClient) getSubnetByName(
	ctx context.Context,
	subnetName string,
	opts GetSubnetOpts,
) (*Subnet, error) {
	filter := fmt.Sprintf(`name eq '%s'`, subnetName)
	if opts.Cluster != "" {
		apiCluster, err := n.client.Cluster().GetCluster(ctx, opts.Cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster %s: %w", opts.Cluster, err)
		}

		filter += fmt.Sprintf(` and clusterReference eq '%s'`, apiCluster.ExtID())
	}

	apiSubnets, err := n.v4Client.Subnets.List(
		ctx,
		converged.WithFilter(filter),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find subnet uuid for subnet %q: %w",
			subnetName,
			err,
		)
	}
	if apiSubnets == nil {
		return nil, fmt.Errorf("no subnet found with name %q", subnetName)
	}

	if len(apiSubnets) == 0 {
		return nil, fmt.Errorf("no subnet found with name %q", subnetName)
	}
	if len(apiSubnets) > 1 {
		return nil, fmt.Errorf(
			"multiple subnets (%d) found with name %q",
			len(apiSubnets),
			subnetName,
		)
	}
	if apiSubnets[0].ExtId == nil {
		return nil, fmt.Errorf("no extID found for subnet %q", subnetName)
	}
	extID := *apiSubnets[0].ExtId
	subnetUUID, err := uuid.Parse(extID)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse subnet uuid %q for cluster %q: %w",
			extID,
			opts.Cluster,
			err,
		)
	}

	return n.getSubnetByExtID(ctx, subnetUUID)
}

func (n *networkingClient) getSubnetByExtID(
	ctx context.Context,
	subnetExtID uuid.UUID,
) (*Subnet, error) {
	apiSubnet, err := n.v4Client.Subnets.Get(ctx, subnetExtID.String())
	if err != nil {
		return nil, fmt.Errorf(
			"failed to find subnet with extID %q: %w",
			subnetExtID,
			err,
		)
	}
	if apiSubnet == nil {
		return nil, fmt.Errorf("no subnet found with extID %q", subnetExtID)
	}

	if apiSubnet.ExtId == nil {
		return nil, fmt.Errorf("no extID found for subnet %q", subnetExtID)
	}
	return newSubnetFromAPI(apiSubnet, fmt.Sprintf("subnet %q", subnetExtID))
}

func newSubnetFromAPI(apiSubnet *networkingapi.Subnet, description string) (*Subnet, error) {
	if apiSubnet.ExtId == nil {
		return nil, fmt.Errorf("no extID found for %s", description)
	}
	subnetUUID, err := uuid.Parse(*apiSubnet.ExtId)
	if err != nil {
		return nil,
			fmt.Errorf(
				"failed to parse subnet extID %q for %s: %w",
				*apiSubnet.ExtId,
				description,
				err,
			)
	}
	prefix, err := subnetPrefix(apiSubnet, description)
	if err != nil {
		return nil, err
	}

	return NewSubnet(subnetUUID, prefix), nil
}

func subnetPrefix(apiSubnet *networkingapi.Subnet, description string) (int32, error) {
	if apiSubnet.IpPrefix != nil {
		prefix, err := netip.ParsePrefix(*apiSubnet.IpPrefix)
		if err != nil {
			return 0, fmt.Errorf(
				"failed to parse IP prefix %q for %s: %w",
				*apiSubnet.IpPrefix,
				description,
				err,
			)
		}
		return prefixLengthToInt32(prefix.Bits(), description)
	}
	for _, ipConfig := range apiSubnet.IpConfig {
		if ipConfig.Ipv4 != nil && ipConfig.Ipv4.IpSubnet != nil &&
			ipConfig.Ipv4.IpSubnet.PrefixLength != nil {
			return prefixLengthToInt32(*ipConfig.Ipv4.IpSubnet.PrefixLength, description)
		}
		if ipConfig.Ipv6 != nil && ipConfig.Ipv6.IpSubnet != nil &&
			ipConfig.Ipv6.IpSubnet.PrefixLength != nil {
			return prefixLengthToInt32(*ipConfig.Ipv6.IpSubnet.PrefixLength, description)
		}
	}
	return 0, fmt.Errorf("no IP prefix found for %s", description)
}

func prefixLengthToInt32(prefixLength int, description string) (int32, error) {
	if prefixLength < 0 || prefixLength > 128 {
		return 0, fmt.Errorf("invalid IP prefix length %d for %s", prefixLength, description)
	}
	return int32(prefixLength), nil
}
