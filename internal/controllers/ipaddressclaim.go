// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipampredicates "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/predicates"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prismgoclient "github.com/nutanix-cloud-native/prism-go-client"
	"github.com/nutanix-cloud-native/prism-go-client/adapter"
	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
)

type genericNutanixIPPool interface {
	ctrlclient.Object
	PoolSpec() *v1alpha1.NutanixIPPoolSpec
}

// NutanixProviderAdapter is used as middle layer for provider integration.
type NutanixProviderAdapter struct {
	k8sClient        ctrlclient.Client
	watchFilterValue string
	pcClientGetter   func(adapter.CachedClientParams) (adapter.Client, error)
}

var _ ipamutil.ProviderAdapter = &NutanixProviderAdapter{}

func NewNutanixProviderAdapter(
	client ctrlclient.Client,
	watchFilter string,
) *NutanixProviderAdapter {
	return &NutanixProviderAdapter{
		k8sClient:        client,
		pcClientGetter:   adapter.GetClient,
		watchFilterValue: watchFilter,
	}
}

// IPAddressClaimHandler reconciles a InClusterIPPool object.
type IPAddressClaimHandler struct {
	ctrlclient.Client
	claim          *ipamv1.IPAddressClaim
	pool           genericNutanixIPPool
	pcClientGetter func(adapter.CachedClientParams) (adapter.Client, error)
}

var _ ipamutil.ClaimHandler = &IPAddressClaimHandler{}

// SetupWithManager sets up the controller with the Manager.
func (i *NutanixProviderAdapter) SetupWithManager(_ context.Context, b *ctrl.Builder) error {
	b.
		For(&ipamv1.IPAddressClaim{}, builder.WithPredicates(
			predicate.Or(
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha1.GroupVersion.Group,
					Kind:  v1alpha1.NutanixIPPoolKind,
				}),
			),
		)).
		Watches(
			&v1alpha1.NutanixIPPool{},
			handler.EnqueueRequestsFromMapFunc(i.ipPoolToIPClaims(v1alpha1.NutanixIPPoolKind)),
			builder.WithPredicates(resourceUnpaused()),
		).
		Owns(&ipamv1.IPAddress{}, builder.WithPredicates(
			ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
				Group: v1alpha1.GroupVersion.Group,
				Kind:  v1alpha1.NutanixIPPoolKind,
			}),
		))
	return nil
}

func (i *NutanixProviderAdapter) ipPoolToIPClaims(
	kind string,
) func(context.Context, ctrlclient.Object) []reconcile.Request {
	return func(ctx context.Context, a ctrlclient.Object) []reconcile.Request {
		pool := a.(genericNutanixIPPool)
		claims := &ipamv1.IPAddressClaimList{}
		err := i.k8sClient.List(ctx, claims,
			ctrlclient.MatchingFields{
				"index.poolRef": index.IPPoolRefValue(corev1.TypedLocalObjectReference{
					Name:     pool.GetName(),
					Kind:     kind,
					APIGroup: &v1alpha1.GroupVersion.Group,
				}),
			},
			ctrlclient.InNamespace(pool.GetNamespace()),
		)
		if err != nil {
			return nil
		}

		return lo.Map(claims.Items, func(claim ipamv1.IPAddressClaim, _ int) reconcile.Request {
			return reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			}
		})
	}
}

// ClaimHandlerFor returns a claim handler for a specific claim.
func (i *NutanixProviderAdapter) ClaimHandlerFor(
	_ ctrlclient.Client,
	claim *ipamv1.IPAddressClaim,
) ipamutil.ClaimHandler {
	return &IPAddressClaimHandler{
		Client:         i.k8sClient,
		claim:          claim,
		pcClientGetter: i.pcClientGetter,
	}
}

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixippools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixippools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixippools/finalizers,verbs=update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// FetchPool fetches the NutanixIPPool.
func (h *IPAddressClaimHandler) FetchPool(
	ctx context.Context,
) (ctrlclient.Object, *ctrl.Result, error) {
	if h.claim.Spec.PoolRef.Kind == v1alpha1.NutanixIPPoolKind {
		h.pool = &v1alpha1.NutanixIPPool{}
		if err := h.Client.Get(
			ctx, types.NamespacedName{Namespace: h.claim.Namespace, Name: h.claim.Spec.PoolRef.Name}, h.pool,
		); err != nil {
			return nil, nil, errors.Wrap(err, "failed to fetch pool")
		}
	}

	return h.pool, nil, nil
}

// EnsureAddress ensures that the IPAddress contains a valid address.
func (h *IPAddressClaimHandler) EnsureAddress(
	ctx context.Context,
	address *ipamv1.IPAddress,
) (*ctrl.Result, error) {
	nutanixClient, err := h.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	// Check if the address already exists.
	err = h.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(address), address)
	// A nil error means the address already exists so nothing to do.
	if err == nil {
		return nil, nil
	}
	// If any other error than NotFound, return the error.
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for existing IPAddress: %w", err)
	}

	// Now actually reserve the IP address.
	reservedIP, err := nutanixClient.Networking().ReserveIP(
		ctx,
		h.pool.PoolSpec().Subnet,
		adapter.ReserveIPOpts{Cluster: ptr.Deref(h.pool.PoolSpec().Cluster, "")},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reserve IP: %w", err)
	}

	address.Spec.Address = reservedIP.String()

	return nil, nil
}

// ReleaseAddress releases the ip address.
func (h *IPAddressClaimHandler) ReleaseAddress(ctx context.Context) (*ctrl.Result, error) {
	if h.claim.Status.AddressRef.Name == "" {
		return nil, nil
	}

	var address ipamv1.IPAddress
	if err := h.Client.Get(
		ctx,
		ctrlclient.ObjectKey{Namespace: h.pool.GetNamespace(), Name: h.claim.Status.AddressRef.Name},
		&address,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get IPAddress: %w", err)
	}

	nutanixClient, err := h.getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	if err := nutanixClient.Networking().UnreserveIP(
		ctx,
		net.ParseIP(address.Spec.Address),
		h.pool.PoolSpec().Subnet,
		adapter.UnreserveIPOpts{Cluster: ptr.Deref(h.pool.PoolSpec().Cluster, "")},
	); err != nil {
		return nil, fmt.Errorf("failed to unreserve IP: %w", err)
	}

	return nil, nil
}

func resourceUnpaused() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return !annotations.HasPaused(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return annotations.HasPaused(e.ObjectOld) && !annotations.HasPaused(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func (h *IPAddressClaimHandler) getClient(ctx context.Context) (adapter.Client, error) {
	pc := h.pool.PoolSpec().PrismCentral
	var (
		credentialSecret    corev1.Secret
		credentialSecretKey = ctrlclient.ObjectKey{
			Namespace: h.pool.GetNamespace(),
			Name:      pc.CredentialsSecretRef.Name,
		}
	)
	if err := h.Client.Get(ctx,
		credentialSecretKey,
		&credentialSecret,
	); err != nil {
		return nil, fmt.Errorf(
			"failed to fetch credentials secret %v: %w",
			credentialSecretKey,
			err,
		)
	}

	// Parse credentials in secret
	credentialData, ok := credentialSecret.Data[credentials.KeyName]
	if !ok {
		return nil, fmt.Errorf(
			"no %q data found in secret %v",
			credentials.KeyName,
			credentialSecretKey,
		)
	}

	apiCredential, err := credentials.ParseCredentials(credentialData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	cacheClientParams, err := newClientCacheParams(h.pool, &prismgoclient.Credentials{
		Username:    apiCredential.Username,
		Password:    apiCredential.Password,
		Endpoint:    pc.Address,
		Port:        strconv.Itoa(int(pc.Port)),
		Insecure:    false,
		SessionAuth: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Nutanix cache client params: %w", err)
	}

	c, err := h.pcClientGetter(cacheClientParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	return c, nil
}
