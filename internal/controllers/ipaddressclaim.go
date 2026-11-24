// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
	"golang.org/x/time/rate"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipampredicates "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/predicates"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
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
	pcClientGetter   func(pcclient.CachedClientParams) (pcclient.Client, error)
	secretInformer   coreinformers.SecretInformer
	cmInformer       coreinformers.ConfigMapInformer
	opts             reconcilerOptions
}

var _ ipamutil.ProviderAdapter = &NutanixProviderAdapter{}

type reconcilerOptions struct {
	maxConcurrentReconciles        int
	minRequeueTime, maxRequeueTime time.Duration
}

func DefaultReconcilerOptions() reconcilerOptions {
	return reconcilerOptions{
		maxConcurrentReconciles: 10,
		minRequeueTime:          500 * time.Millisecond,
		maxRequeueTime:          1 * time.Minute,
	}
}

func (o *reconcilerOptions) AddFlags(fs *pflag.FlagSet) {
	fs.IntVar(
		&o.maxConcurrentReconciles,
		"max-concurrent-reconciles",
		o.maxConcurrentReconciles,
		"Number of reconciles to run concurrently",
	)
	fs.DurationVar(
		&o.minRequeueTime,
		"min-requeue-delay",
		o.minRequeueTime,
		"Minimum time to wait when requeueing on error",
	)
	fs.DurationVar(
		&o.maxRequeueTime,
		"max-requeue-delay",
		o.maxRequeueTime,
		"Maximum time to wait when requeueing on error",
	)
}

func NewNutanixProviderAdapter(
	client ctrlclient.Client,
	watchFilter string,
	secretInformer coreinformers.SecretInformer,
	cmInformer coreinformers.ConfigMapInformer,
	opts reconcilerOptions,
) *NutanixProviderAdapter {
	return &NutanixProviderAdapter{
		k8sClient:        client,
		pcClientGetter:   pcclient.GetClient,
		watchFilterValue: watchFilter,
		secretInformer:   secretInformer,
		cmInformer:       cmInformer,
		opts:             opts,
	}
}

// IPAddressClaimHandler reconciles a NutanixIPPool object.
type IPAddressClaimHandler struct {
	client         ctrlclient.Client
	claim          *ipamv1.IPAddressClaim
	pool           genericNutanixIPPool
	pcClientGetter func(pcclient.CachedClientParams) (pcclient.Client, error)
	secretInformer coreinformers.SecretInformer
	cmInformer     coreinformers.ConfigMapInformer
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
		)).
		WithOptions(
			controller.Options{
				MaxConcurrentReconciles: i.opts.maxConcurrentReconciles,
				RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
					workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
						i.opts.minRequeueTime,
						i.opts.maxRequeueTime,
					),
					// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
					&workqueue.TypedBucketRateLimiter[reconcile.Request]{
						Limiter: rate.NewLimiter(rate.Limit(10), 100),
					},
				),
			},
		)
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
				"index.poolRef": index.IPPoolRefValue(ipamv1.IPPoolReference{
					Name:     pool.GetName(),
					Kind:     kind,
					APIGroup: v1alpha1.GroupVersion.Group,
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
		client:         i.k8sClient,
		claim:          claim,
		pcClientGetter: i.pcClientGetter,
		secretInformer: i.secretInformer,
		cmInformer:     i.cmInformer,
	}
}

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixippools,verbs=get;list;watch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixippools/finalizers,verbs=update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/finalizers;ipaddresses/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch

// FetchPool fetches the NutanixIPPool.
func (h *IPAddressClaimHandler) FetchPool(
	ctx context.Context,
) (ctrlclient.Object, *ctrl.Result, error) {
	if h.claim.Spec.PoolRef.Kind == v1alpha1.NutanixIPPoolKind {
		h.pool = &v1alpha1.NutanixIPPool{}
		if err := h.client.Get(
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
	// An address already carrying both the reserved address and the prefix is
	// complete, so there is nothing to do and no reason to call Prism Central.
	// Anything less is finished below: returning early on the address alone
	// left prefix and gateway unset for good, because this is the only place
	// that fills them in. Measured against a live /25 subnet: the allocated
	// IPAddress reported prefix 0 and no gateway.
	existing := &ipamv1.IPAddress{}
	err := h.client.Get(ctx, ctrlclient.ObjectKeyFromObject(address), existing)
	switch {
	case err == nil:
		if existing.Spec.Address != "" && existing.Spec.Prefix != nil &&
			*existing.Spec.Prefix != 0 {
			return nil, nil
		}
	case !apierrors.IsNotFound(err):
		return nil, fmt.Errorf("failed to check for existing IPAddress: %w", err)
	}

	nutanixClient, err := h.getClient()
	if err != nil {
		return h.recordFailure(ctx, ReasonClientUnavailable,
			fmt.Errorf("failed to get Nutanix client: %w", err))
	}
	cluster := ptr.Deref(h.pool.PoolSpec().Cluster, "")
	subnet, err := nutanixClient.Networking().GetSubnet(
		ctx,
		h.pool.PoolSpec().Subnet,
		pcclient.GetSubnetOpts{Cluster: cluster},
	)
	if err != nil {
		return h.recordFailure(ctx, ReasonSubnetUnresolved,
			fmt.Errorf("failed to get subnet: %w", err))
	}

	// Reuse the address already reserved for this claim rather than reserving a
	// second one just to fill in a missing prefix.
	reserved := existing.Spec.Address
	if reserved == "" {
		// Reserve the IP address. This blocks until the underlying Prism task
		// completes and returns the reserved IPs.
		reservedIPs, reserveErr := nutanixClient.Networking().ReserveIPs(
			ctx,
			pcclient.ReserveIPCountFunc(1),
			h.pool.PoolSpec().Subnet,
			pcclient.ReserveIPOpts{
				Cluster:       cluster,
				ClientContext: string(h.claim.UID),
			},
		)
		if reserveErr != nil {
			return h.reservationFailed(ctx, reserveErr)
		}
		reserved = reservedIPs[0].String()
	}

	address.Spec.Address = reserved
	address.Spec.Prefix = ptr.To(subnet.Prefix())
	if gateway := subnet.Gateway(); gateway != "" {
		address.Spec.Gateway = gateway
	}

	meta.SetStatusCondition(&h.claim.Status.Conditions, metav1.Condition{
		Type:    ReadyCondition,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonAddressReserved,
		Message: fmt.Sprintf("Reserved %s on subnet %s", reserved, h.pool.PoolSpec().Subnet),
	})

	return nil, nil
}

// Condition types and reasons recorded on an IPAddressClaim.
//
// Nothing used to be recorded at all, so a reservation that could not succeed
// was visible only in this controller's log. Anything watching the claim - an
// operator, or the CAPI lifecycle hook that waits for the address before a
// cluster is created - saw a claim with no address and no reason, which is
// indistinguishable from one that is merely slow.
const (
	ReadyCondition = "Ready"

	ReasonAddressReserved = "AddressReserved"
	ReasonPoolExhausted   = "SubnetPoolExhausted"
	ReasonRateLimited     = "PrismCentralRateLimited"
	ReasonReserveFailed   = "ReservationFailed"
	//nolint:gosec // G101 matches the name, but this is a condition reason, not a credential.
	ReasonCredentialsGone = "PrismCentralCredentialsGone"
	// ReasonSubnetUnresolved covers a subnet that cannot be resolved: absent,
	// or a name matching none or several subnets.
	ReasonSubnetUnresolved = "SubnetUnresolved"
	// ReasonClientUnavailable covers not being able to build a Prism Central
	// client at all, which is usually a missing or unreadable credentials Secret.
	ReasonClientUnavailable = "PrismCentralClientUnavailable"
)

// reservationFailed records why a reservation failed.
func (h *IPAddressClaimHandler) reservationFailed(
	ctx context.Context,
	err error,
) (*ctrl.Result, error) {
	return h.recordFailure(ctx, classifyReserveError(err), err)
}

// recordFailure puts the cause on the claim and returns the error.
//
// Every failure on the path to an address goes through here, not just the
// reserve call: resolving the subnet and building the Prism Central client come
// before it, and used to return bare errors, so a claim against an absent
// subnet carried no condition at all - the "no address and no reason" state the
// condition exists to remove.
//
// The error is returned rather than swapped for a RequeueAfter, because
// EnsureAddress runs inside the upstream reconciler's CreateOrPatch mutate
// function (cluster-api-ipam-provider-in-cluster, pkg/ipamutil). Returning nil
// there lets it go on to persist an IPAddress that carries no address yet,
// which the API server rejects - measured as "failed to create or patch
// address: IPAddress ... invalid" - and controller-runtime discards a
// RequeueAfter whenever an error comes back with it. Retry pacing therefore
// belongs to the controller's rate limiter (--min/--max-requeue-time), not
// here; an error is the only way to stop an incomplete address being written.
func (h *IPAddressClaimHandler) recordFailure(
	ctx context.Context,
	reason string,
	err error,
) (*ctrl.Result, error) {
	meta.SetStatusCondition(&h.claim.Status.Conditions, metav1.Condition{
		Type:    ReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: err.Error(),
	})
	log.FromContext(ctx).Info(
		"Could not reserve an address", "reason", reason, "error", err.Error(),
	)
	return nil, err
}

// classifyReserveError names the failure, for the condition an operator reads.
// Every reservation failure is waited out, so this only chooses the label.
//
// Prism Central reports these inside a task or error payload rather than as a
// typed error, so they are matched on the codes it returns:
// kAddressPoolExhausted for a full subnet and PLAT-10003 for the rate limit.
//
// Anything else is reported as a plain failure. When the reserve call fails as an
// asynchronous task, the only text that reaches here is the task's generic
// "Operation failed due to legacy error" - the code naming the cause lives in
// the task's legacyErrorMessage, which the client does not surface. Measured
// on PC 7.6 GA: an exhausted subnet arrives that way, so matching on the code
// alone classified real exhaustion as unknown, and treating unknown as
// retry-hard is what put this controller into a one-attempt-per-second loop
// against a shared Prism Central.
func classifyReserveError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "kAddressPoolExhausted"):
		return ReasonPoolExhausted
	case strings.Contains(msg, "PLAT-10003"), strings.Contains(msg, "RATE_LIMIT_EXCEEDED"):
		return ReasonRateLimited
	default:
		return ReasonReserveFailed
	}
}

// ReleaseAddress releases the ip address.
func (h *IPAddressClaimHandler) ReleaseAddress(ctx context.Context) (*ctrl.Result, error) {
	if h.claim.Status.AddressRef.Name == "" {
		return nil, nil
	}

	// The reconciler calls this even when FetchPool failed, which the
	// ClaimHandler contract spells out, so the pool may be nil. Everything
	// below needs it: it carries the Prism Central to talk to and the subnet to
	// release on. Returning an error keeps the claim's finalizer in place and
	// retries, rather than dereferencing nil and dropping the address on the
	// floor. ProtectPoolFinalizer is what stops this happening at all.
	if h.pool == nil {
		return nil, fmt.Errorf(
			"cannot release the address for IPAddressClaim %s/%s: its NutanixIPPool %q is gone, "+
				"so the Prism Central and subnet to release on are unknown",
			h.claim.Namespace, h.claim.Name, h.claim.Spec.PoolRef.Name,
		)
	}

	var address ipamv1.IPAddress
	if err := h.client.Get(
		ctx,
		ctrlclient.ObjectKey{Namespace: h.pool.GetNamespace(), Name: h.claim.Status.AddressRef.Name},
		&address,
	); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get IPAddress: %w", err)
	}

	nutanixClient, err := h.getClient()
	if err != nil {
		// The Prism Central credentials live in a Secret in the cluster's
		// namespace, which is deleted along with the cluster. Once it is gone
		// this controller can never build a client again, so retrying cannot
		// release the address: it only wedges the claim, the pool holding it
		// open and the namespace behind a finalizer that will never clear.
		//
		// Observed on a live cluster: the secret went with the cluster and the
		// claim then retried "secret not found" every ten seconds forever.
		//
		// Record it, say plainly that the address may still be reserved, and
		// let the object go. CAREN's cluster-level sweep runs during
		// BeforeClusterDelete, while the secret still exists, and is the
		// designed backstop for exactly this.
		if apierrors.IsNotFound(err) {
			meta.SetStatusCondition(&h.claim.Status.Conditions, metav1.Condition{
				Type:   ReadyCondition,
				Status: metav1.ConditionFalse,
				Reason: ReasonCredentialsGone,
				Message: fmt.Sprintf(
					"Cannot release the address: %v. It may remain reserved on Prism Central.",
					err,
				),
			})
			log.FromContext(ctx).Error(err,
				"Prism Central credentials are gone, so the address cannot be released here; "+
					"allowing the claim to be deleted rather than blocking it forever",
				"claim", fmt.Sprintf("%s/%s", h.claim.Namespace, h.claim.Name),
				"clientContext", string(h.claim.UID),
			)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	// Unreserve the IP address by client context. This blocks until the
	// underlying Prism task completes and is idempotent if the IP was already
	// released. Unreserving by context means the server resolves and releases
	// the IPs, so the returned list is the authoritative record of what was
	// freed.
	unreservedIPs, err := nutanixClient.Networking().UnreserveIPs(
		ctx,
		pcclient.UnreserveIPClientContext(string(h.claim.UID)),
		h.pool.PoolSpec().Subnet,
		pcclient.UnreserveIPOpts{
			Cluster: ptr.Deref(h.pool.PoolSpec().Cluster, ""),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to unreserve IP: %w", err)
	}

	log.FromContext(ctx).V(1).Info(
		"Unreserved IP addresses by client context",
		"clientContext", string(h.claim.UID),
		"unreservedIPs", unreservedIPs,
	)

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

func (h *IPAddressClaimHandler) getClient() (pcclient.Client, error) {
	pc := h.pool.PoolSpec().PrismCentral

	var additionalTrustBundle *credentials.NutanixTrustBundleReference
	if pc.AdditionalTrustBundle != nil {
		switch {
		case len(pc.AdditionalTrustBundle.Data) > 0:
			additionalTrustBundle = &credentials.NutanixTrustBundleReference{
				Data: string(pc.AdditionalTrustBundle.Data),
				Kind: credentials.NutanixTrustBundleKindString,
			}
		case pc.AdditionalTrustBundle.ConfigMapReference != nil && pc.AdditionalTrustBundle.ConfigMapReference.Name != "":
			additionalTrustBundle = &credentials.NutanixTrustBundleReference{
				Name:      pc.AdditionalTrustBundle.ConfigMapReference.Name,
				Namespace: h.pool.GetNamespace(),
				Kind:      credentials.NutanixTrustBundleKindConfigMap,
			}
		default:
			return nil, fmt.Errorf(
				"invalid additional trust bundle configuration: either data or secretRef must be set",
			)
		}
	}

	cacheClientParams, err := newClientCacheParams(
		credentials.NutanixPrismEndpoint{
			Address:               pc.Address,
			Port:                  int32(pc.Port),
			Insecure:              pc.Insecure,
			AdditionalTrustBundle: additionalTrustBundle,
			CredentialRef: &credentials.NutanixCredentialReference{
				Kind:      credentials.SecretKind,
				Name:      pc.CredentialsSecretRef.Name,
				Namespace: h.pool.GetNamespace(),
			},
		},
		h.secretInformer,
		h.cmInformer,
		h.pool,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nutanix cache client params: %w", err)
	}

	c, err := h.pcClientGetter(cacheClientParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	return c, nil
}
