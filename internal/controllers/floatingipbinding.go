// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/util/workqueue"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha2 "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
)

const floatingIPBindingRequeue = time.Second

// FloatingIPBindingReconciler reconciles FloatingIPBinding objects.
type FloatingIPBindingReconciler struct {
	ctrlclient.Client
	Scheme            *runtime.Scheme
	WatchFilterValue  string
	PCClientGetter    func(pcclient.CachedClientParams) (pcclient.Client, error)
	SecretInformer    coreinformers.SecretInformer
	ConfigMapInformer coreinformers.ConfigMapInformer
	Opts              reconcilerOptions
}

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipbindings,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingips,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingips/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingips/finalizers,verbs=update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixfloatingippools,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch

func (r *FloatingIPBindingReconciler) SetupWithManager(
	ctx context.Context,
	mgr ctrl.Manager,
) error {
	if r.PCClientGetter == nil {
		r.PCClientGetter = pcclient.GetClient
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.FloatingIPBinding{}, builder.WithPredicates(
			predicates.ResourceNotPausedAndHasFilterLabel(
				mgr.GetScheme(),
				ctrl.LoggerFrom(ctx),
				r.WatchFilterValue,
			),
		)).
		Watches(
			&v1alpha2.FloatingIP{},
			handler.EnqueueRequestsFromMapFunc(r.floatingIPToBindings),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.Opts.maxConcurrentReconciles,
			RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
				workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
					r.Opts.minRequeueTime,
					r.Opts.maxRequeueTime,
				),
			),
		}).
		Complete(r)
}

func (r *FloatingIPBindingReconciler) floatingIPToBindings(
	ctx context.Context,
	obj ctrlclient.Object,
) []reconcile.Request {
	fip, ok := obj.(*v1alpha2.FloatingIP)
	if !ok {
		return nil
	}
	bindings := &v1alpha2.FloatingIPBindingList{}
	if err := r.List(ctx, bindings,
		ctrlclient.InNamespace(fip.Namespace),
		ctrlclient.MatchingFields{index.FloatingIPBindingFloatingIPRefField: fip.Name},
	); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(bindings.Items))
	for i := range bindings.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: ctrlclient.ObjectKeyFromObject(&bindings.Items[i]),
		})
	}
	return reqs
}

func (r *FloatingIPBindingReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (_ ctrl.Result, reterr error) {
	log := log.FromContext(ctx)

	binding := &v1alpha2.FloatingIPBinding{}
	if err := r.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	paused, err := isObjectOrClusterPaused(ctx, r.Client, binding, "")
	if err != nil {
		return ctrl.Result{}, err
	}
	if paused {
		log.Info("FloatingIPBinding is paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(binding, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, binding); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if hasDeleteForMove(binding) {
		controllerutil.RemoveFinalizer(binding, v1alpha2.BoundFinalizer)
		return ctrl.Result{}, nil
	}

	if binding.Annotations[v1alpha2.BindingIDAnnotation] == "" {
		setAnnotation(binding, v1alpha2.BindingIDAnnotation, uuid.NewString())
		return ctrl.Result{}, nil
	}

	floatingIP := &v1alpha2.FloatingIP{}
	if err := r.Get(ctx, ctrlclient.ObjectKey{
		Namespace: binding.Namespace,
		Name:      binding.Spec.FloatingIPRef.Name,
	}, floatingIP); err != nil {
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPBindingAssociationFailedReason,
			err.Error(),
		)
		if apierrors.IsNotFound(err) {
			if !binding.DeletionTimestamp.IsZero() {
				controllerutil.RemoveFinalizer(binding, v1alpha2.BoundFinalizer)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !binding.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, binding, floatingIP)
	}

	return r.reconcileNormal(ctx, binding, floatingIP)
}

func (r *FloatingIPBindingReconciler) reconcileNormal(
	ctx context.Context,
	binding *v1alpha2.FloatingIPBinding,
	floatingIP *v1alpha2.FloatingIP,
) (ctrl.Result, error) {
	if floatingIP.Spec.ExtID == "" {
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPBindingAssociationPendingReason,
			"FloatingIP is not allocated yet",
		)
		return ctrl.Result{RequeueAfter: floatingIPBindingRequeue}, nil
	}

	bindingID := binding.Annotations[v1alpha2.BindingIDAnnotation]
	reserved := floatingIP.Status.Reservation

	if reserved != nil && reserved.BindingID != bindingID {
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPBindingAssociationConflictReason,
			fmt.Sprintf("FloatingIP is reserved by binding %q", reserved.Name),
		)
		return ctrl.Result{}, nil
	}

	if reserved == nil {
		eligible, err := r.eligibleBindings(ctx, floatingIP)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(eligible) > 1 {
			setCondition(
				&binding.Status.Conditions,
				v1alpha2.FloatingIPBindingReadyCondition,
				metav1.ConditionFalse,
				v1alpha2.FloatingIPBindingAssociationConflictReason,
				"Multiple FloatingIPBindings compete for an unreserved FloatingIP",
			)
			return ctrl.Result{}, nil
		}
		if len(eligible) == 1 && eligible[0].Name == binding.Name {
			// Requeue briefly so concurrently-created sibling bindings can appear in
			// the informer before this binding claims the reservation.
			if time.Since(binding.CreationTimestamp.Time) < 2*time.Second {
				return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
			}
			if err := r.reserveFloatingIP(ctx, floatingIP, binding); err != nil {
				if apierrors.IsConflict(err) {
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{}, err
			}
			if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), floatingIP); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			setCondition(
				&binding.Status.Conditions,
				v1alpha2.FloatingIPBindingReadyCondition,
				metav1.ConditionFalse,
				v1alpha2.FloatingIPBindingAssociationConflictReason,
				"FloatingIP reservation was not acquired",
			)
			return ctrl.Result{}, nil
		}
	}

	controllerutil.AddFinalizer(binding, v1alpha2.BoundFinalizer)
	if !controllerutil.ContainsFinalizer(floatingIP, v1alpha2.BoundFloatingIPFinalizer) {
		controllerutil.AddFinalizer(floatingIP, v1alpha2.BoundFloatingIPFinalizer)
		if err := r.Update(ctx, floatingIP); err != nil {
			return ctrl.Result{}, err
		}
		// Re-fetch so a subsequent status update does not wipe reservation.
		if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), floatingIP); err != nil {
			return ctrl.Result{}, err
		}
	}

	return r.ensureAssociation(ctx, binding, floatingIP)
}

func (r *FloatingIPBindingReconciler) eligibleBindings(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
) ([]v1alpha2.FloatingIPBinding, error) {
	bindings := &v1alpha2.FloatingIPBindingList{}
	if err := r.List(ctx, bindings,
		ctrlclient.InNamespace(floatingIP.Namespace),
		ctrlclient.MatchingFields{index.FloatingIPBindingFloatingIPRefField: floatingIP.Name},
	); err != nil {
		return nil, err
	}
	eligible := make([]v1alpha2.FloatingIPBinding, 0, len(bindings.Items))
	for i := range bindings.Items {
		b := bindings.Items[i]
		if b.DeletionTimestamp.IsZero() {
			eligible = append(eligible, b)
		}
	}
	return eligible, nil
}

func (r *FloatingIPBindingReconciler) reserveFloatingIP(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
	binding *v1alpha2.FloatingIPBinding,
) error {
	latest := &v1alpha2.FloatingIP{}
	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); err != nil {
		return err
	}
	if latest.Status.Reservation != nil {
		return nil
	}
	eligible, err := r.eligibleBindings(ctx, latest)
	if err != nil {
		return err
	}
	if len(eligible) != 1 || eligible[0].Name != binding.Name {
		return nil
	}

	latest.Status.Reservation = &v1alpha2.FloatingIPReservation{
		Name:      binding.Name,
		BindingID: binding.Annotations[v1alpha2.BindingIDAnnotation],
	}
	return r.Status().Update(ctx, latest)
}

func (r *FloatingIPBindingReconciler) ensureAssociation(
	ctx context.Context,
	binding *v1alpha2.FloatingIPBinding,
	floatingIP *v1alpha2.FloatingIP,
) (ctrl.Result, error) {
	pcClient, pool, err := r.pcClientForFloatingIP(ctx, floatingIP)
	if err != nil {
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPBindingAssociationFailedReason,
			err.Error(),
		)
		return ctrl.Result{}, err
	}
	_ = pool

	current, err := pcClient.FloatingIPs().Get(ctx, floatingIP.Spec.ExtID)
	if err != nil {
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPBindingAssociationFailedReason,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	desiredType, desiredPrivateIP, desiredVMNic := desiredAssociation(binding)
	if associationMatches(current, desiredType, desiredPrivateIP, desiredVMNic) {
		if err := r.patchFloatingIPAssociation(ctx, floatingIP, current); err != nil {
			return ctrl.Result{}, err
		}
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionTrue,
			"Associated",
			"Floating IP associated",
		)
		return ctrl.Result{}, nil
	}

	setCondition(
		&binding.Status.Conditions,
		v1alpha2.FloatingIPBindingReadyCondition,
		metav1.ConditionFalse,
		v1alpha2.FloatingIPBindingAssociationPendingReason,
		"Updating Prism association",
	)

	var updated *pcclient.FloatingIP
	switch desiredType {
	case v1alpha2.FloatingIPAssociationPrivateIP:
		updated, err = pcClient.FloatingIPs().
			AssignPrivateIP(ctx, floatingIP.Spec.ExtID, *desiredPrivateIP)
	case v1alpha2.FloatingIPAssociationVMNic:
		updated, err = pcClient.FloatingIPs().AssignVMNic(ctx, floatingIP.Spec.ExtID, *desiredVMNic)
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported association type %q", desiredType)
	}
	if err != nil {
		setCondition(
			&binding.Status.Conditions,
			v1alpha2.FloatingIPBindingReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPBindingAssociationFailedReason,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	if err := r.patchFloatingIPAssociation(ctx, floatingIP, updated); err != nil {
		return ctrl.Result{}, err
	}
	setCondition(
		&binding.Status.Conditions,
		v1alpha2.FloatingIPBindingReadyCondition,
		metav1.ConditionTrue,
		"Associated",
		"Floating IP associated",
	)
	return ctrl.Result{}, nil
}

func (r *FloatingIPBindingReconciler) patchFloatingIPAssociation(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
	current *pcclient.FloatingIP,
) error {
	latest := &v1alpha2.FloatingIP{}
	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); err != nil {
		return err
	}
	latest.Status.Association = associationStatusFromClient(current)
	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}
	floatingIP.Status = latest.Status
	return nil
}

func desiredAssociation(
	binding *v1alpha2.FloatingIPBinding,
) (v1alpha2.FloatingIPAssociationType, *pcclient.PrivateIPAssociation, *pcclient.VMNicAssociation) {
	if binding.Spec.PrivateIP != nil {
		return v1alpha2.FloatingIPAssociationPrivateIP, &pcclient.PrivateIPAssociation{
			Address:  binding.Spec.PrivateIP.Address,
			VPCExtID: binding.Spec.PrivateIP.VPC.String(),
		}, nil
	}
	if binding.Spec.VMNic != nil {
		return v1alpha2.FloatingIPAssociationVMNic, nil, &pcclient.VMNicAssociation{
			VMNicExtID: binding.Spec.VMNic.String(),
		}
	}
	return v1alpha2.FloatingIPAssociationNotAssociated, nil, nil
}

func associationMatches(
	current *pcclient.FloatingIP,
	desiredType v1alpha2.FloatingIPAssociationType,
	privateIP *pcclient.PrivateIPAssociation,
	vmNic *pcclient.VMNicAssociation,
) bool {
	if current == nil {
		return false
	}
	switch desiredType {
	case v1alpha2.FloatingIPAssociationPrivateIP:
		return current.AssociationType == pcclient.FloatingIPAssociationPrivateIP &&
			privateIP != nil &&
			current.PrivateIPAddress == privateIP.Address &&
			current.PrivateIPVPCExtID == privateIP.VPCExtID
	case v1alpha2.FloatingIPAssociationVMNic:
		return current.AssociationType == pcclient.FloatingIPAssociationVMNic &&
			vmNic != nil &&
			current.VMNicExtID == vmNic.VMNicExtID
	default:
		return current.AssociationType == pcclient.FloatingIPAssociationNone
	}
}

func (r *FloatingIPBindingReconciler) reconcileDelete(
	ctx context.Context,
	binding *v1alpha2.FloatingIPBinding,
	floatingIP *v1alpha2.FloatingIP,
) (ctrl.Result, error) {
	bindingID := binding.Annotations[v1alpha2.BindingIDAnnotation]
	reserved := floatingIP.Status.Reservation
	isHolder := reserved != nil && reserved.BindingID == bindingID
	if !isHolder {
		if err := r.ensureFloatingIPUnbound(ctx, floatingIP, binding); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(binding, v1alpha2.BoundFinalizer)
		return ctrl.Result{}, nil
	}

	var current *pcclient.FloatingIP
	if floatingIP.Spec.ExtID != "" {
		pcClient, _, err := r.pcClientForFloatingIP(ctx, floatingIP)
		if err != nil {
			return ctrl.Result{}, err
		}
		current, err = pcClient.FloatingIPs().Get(ctx, floatingIP.Spec.ExtID)
		if err != nil && !isNotFoundError(err) {
			return ctrl.Result{}, err
		}
		if err == nil && current.AssociationType != pcclient.FloatingIPAssociationNone {
			if _, err := pcClient.FloatingIPs().Deassign(ctx, floatingIP.Spec.ExtID); err != nil {
				return ctrl.Result{}, err
			}
			current, err = pcClient.FloatingIPs().Get(ctx, floatingIP.Spec.ExtID)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if err == nil && current.AssociationType != pcclient.FloatingIPAssociationNone {
			return ctrl.Result{RequeueAfter: floatingIPBindingRequeue}, nil
		}
		if err != nil {
			current = nil
		}
	}

	if err := r.clearFloatingIPReservation(ctx, floatingIP, current); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(binding, v1alpha2.BoundFinalizer)
	return ctrl.Result{}, nil
}

func (r *FloatingIPBindingReconciler) clearFloatingIPReservation(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
	current *pcclient.FloatingIP,
) error {
	latest := &v1alpha2.FloatingIP{}
	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); err != nil {
		return err
	}
	if current != nil {
		latest.Status.Association = associationStatusFromClient(current)
	} else {
		latest.Status.Association = &v1alpha2.FloatingIPAssociationStatus{
			Type: v1alpha2.FloatingIPAssociationNotAssociated,
		}
	}
	latest.Status.Reservation = nil
	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}

	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); err != nil {
		return err
	}
	if controllerutil.RemoveFinalizer(latest, v1alpha2.BoundFloatingIPFinalizer) {
		return r.Update(ctx, latest)
	}
	return nil
}

func (r *FloatingIPBindingReconciler) ensureFloatingIPUnbound(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
	binding *v1alpha2.FloatingIPBinding,
) error {
	if !controllerutil.ContainsFinalizer(binding, v1alpha2.BoundFinalizer) {
		return nil
	}
	if floatingIP.Status.Reservation != nil {
		return nil
	}
	latest := &v1alpha2.FloatingIP{}
	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); err != nil {
		return err
	}
	if !controllerutil.RemoveFinalizer(latest, v1alpha2.BoundFloatingIPFinalizer) {
		return nil
	}
	return r.Update(ctx, latest)
}

func (r *FloatingIPBindingReconciler) pcClientForFloatingIP(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
) (pcclient.Client, *v1alpha2.NutanixFloatingIPPool, error) {
	pool := &v1alpha2.NutanixFloatingIPPool{}
	if err := r.Get(ctx, ctrlclient.ObjectKey{
		Namespace: floatingIP.Namespace,
		Name:      floatingIP.Spec.PoolRef.Name,
	}, pool); err != nil {
		return nil, nil, err
	}
	if annotations.HasPaused(pool) {
		return nil, nil, errors.New("referenced NutanixFloatingIPPool is paused")
	}
	pcClient, err := getPCClientForPrismCentral(
		pool.Spec.PrismCentral,
		pool.Namespace,
		pool,
		r.SecretInformer,
		r.ConfigMapInformer,
		r.PCClientGetter,
	)
	if err != nil {
		return nil, nil, err
	}
	return pcClient, pool, nil
}

// Silence unused import if clusterctl annotation helpers change.
var _ = clusterctlv1.DeleteForMoveAnnotation
