// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/spf13/pflag"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipampredicates "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/predicates"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
)

const (
	reserveIPRequestIDAnnotationKey   = "ipam.cluster.x-k8s.io/ntnx-reserve-ip-request-id"
	unreserveIPRequestIDAnnotationKey = "ipam.cluster.x-k8s.io/ntnx-unreserve-ip-request-id"
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
				RateLimiter: workqueue.NewMaxOfRateLimiter(
					workqueue.NewItemExponentialFailureRateLimiter(
						i.opts.minRequeueTime,
						i.opts.maxRequeueTime,
					),
					// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
					&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
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
	// Check if the address already exists.
	err := h.client.Get(ctx, ctrlclient.ObjectKeyFromObject(address), address)
	// A nil error means the address already exists so nothing to do.
	if err == nil {
		return nil, nil
	}
	// If any other error than NotFound, return the error.
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for existing IPAddress: %w", err)
	}

	reqID, err := ensureRequestIDAnnotation(ctx, h.claim, reserveIPRequestIDAnnotationKey, h.client)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to patch IPAddressClaim to add reserve IP request ID annotation: %w",
			err,
		)
	}

	nutanixClient, err := h.getClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	// Now actually reserve the IP address.
	reservedIP, err := nutanixClient.Networking().ReserveIP(
		pcclient.ReserveIPCount(1),
		h.pool.PoolSpec().Subnet,
		pcclient.ReserveIPOpts{
			Cluster: ptr.Deref(h.pool.PoolSpec().Cluster, ""),
			AsyncTaskOpts: pcclient.AsyncTaskOpts{
				RequestID: reqID,
			},
			ClientContext: string(h.claim.UID),
		},
	)
	if err != nil {
		err = fmt.Errorf("failed to reserve IP: %w", err)

		switch {
		case errors.Is(err, pcclient.ErrTaskOngoing):
			return nil, fmt.Errorf("requeuing to wait for task completion: %w", err)
		default:
			if clearReqIDErr := clearRequestIDAnnotation(
				ctx, h.claim, reserveIPRequestIDAnnotationKey, h.client,
			); clearReqIDErr != nil {
				err = kerrors.NewAggregate(
					append(
						[]error{err},
						fmt.Errorf(
							"failed to patch IPAddressClaim to delete reserve IP request ID annotation: %w",
							clearReqIDErr,
						),
					),
				)
			}
		}

		return nil, err
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

	reqID, err := ensureRequestIDAnnotation(
		ctx,
		h.claim,
		unreserveIPRequestIDAnnotationKey,
		h.client,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to patch IPAddressClaim to add unreserve IP request ID annotation: %w",
			err,
		)
	}

	nutanixClient, err := h.getClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Nutanix client: %w", err)
	}

	err = nutanixClient.Networking().UnreserveIP(
		pcclient.UnreserveIPClientContext(string(h.claim.UID)),
		h.pool.PoolSpec().Subnet,
		pcclient.UnreserveIPOpts{
			Cluster: ptr.Deref(h.pool.PoolSpec().Cluster, ""),
			AsyncTaskOpts: pcclient.AsyncTaskOpts{
				RequestID: reqID,
			},
		},
	)
	if err != nil {
		err = fmt.Errorf("failed to unreserve IP: %w", err)

		switch {
		case errors.Is(err, pcclient.ErrTaskOngoing):
			return nil, fmt.Errorf("requeuing to wait for task completion: %w", err)
		default:
			if clearReqIDErr := clearRequestIDAnnotation(
				ctx, h.claim, unreserveIPRequestIDAnnotationKey, h.client,
			); clearReqIDErr != nil {
				err = kerrors.NewAggregate(
					append(
						[]error{err},
						fmt.Errorf(
							"failed to patch IPAddressClaim to delete unreserve IP request ID annotation: %w",
							clearReqIDErr,
						),
					),
				)
			}
		}

		return nil, err
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

func ensureRequestIDAnnotation(
	ctx context.Context,
	obj ctrlclient.Object,
	requestAnnotationKey string,
	cl ctrlclient.Client,
) (string, error) {
	// If the claim does not have a request ID annotation set, then we set one
	// here. This is used so we can send the same reservation request and receive the same task ID back.
	reqID, found := obj.GetAnnotations()[requestAnnotationKey]
	if !found {
		claimPatch := ctrlclient.MergeFrom(obj.DeepCopyObject().(ctrlclient.Object))

		reqUUID, err := uuid.NewV7()
		if err != nil {
			return "", fmt.Errorf("failed to generate request UUID: %w", err)
		}

		reqID = reqUUID.String()

		if obj.GetAnnotations() == nil {
			obj.SetAnnotations(make(map[string]string, 1))
		}

		obj.GetAnnotations()[requestAnnotationKey] = reqID

		if err := cl.Patch(ctx, obj, claimPatch); err != nil {
			return "", fmt.Errorf(
				"failed to patch object with request ID annotation %q: %w",
				requestAnnotationKey, err,
			)
		}
	}

	return reqID, nil
}

func clearRequestIDAnnotation(ctx context.Context,
	obj ctrlclient.Object,
	requestAnnotationKey string,
	cl ctrlclient.Client,
) error {
	objPatch := ctrlclient.MergeFrom(obj.DeepCopyObject().(ctrlclient.Object))

	delete(obj.GetAnnotations(), requestAnnotationKey)

	if err := cl.Patch(ctx, obj, objPatch); err != nil {
		return fmt.Errorf(
			"failed to patch object with request ID annotation %q: %w",
			requestAnnotationKey,
			err,
		)
	}

	return nil
}
