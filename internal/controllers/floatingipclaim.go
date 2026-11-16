// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/util/workqueue"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
)

const floatingIPClaimRequeue = time.Second

// FloatingIPClaimReconciler reconciles FloatingIPClaim objects.
type FloatingIPClaimReconciler struct {
	ctrlclient.Client
	Scheme            *runtime.Scheme
	WatchFilterValue  string
	PCClientGetter    func(pcclient.CachedClientParams) (pcclient.Client, error)
	SecretInformer    coreinformers.SecretInformer
	ConfigMapInformer coreinformers.ConfigMapInformer
	Opts              reconcilerOptions
}

// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipclaims,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipclaims/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingips,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingips/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingips/finalizers,verbs=update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixfloatingippools,verbs=get;list;watch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=floatingipbindings,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;configmaps,verbs=get;list;watch

func (r *FloatingIPClaimReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.PCClientGetter == nil {
		r.PCClientGetter = pcclient.GetClient
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.FloatingIPClaim{}, builder.WithPredicates(
			predicates.ResourceNotPausedAndHasFilterLabel(
				mgr.GetScheme(),
				ctrl.LoggerFrom(ctx),
				r.WatchFilterValue,
			),
		)).
		Owns(&v1alpha2.FloatingIP{}).
		Watches(
			&v1alpha2.NutanixFloatingIPPool{},
			handler.EnqueueRequestsFromMapFunc(r.poolToClaims),
			builder.WithPredicates(resourceUnpaused()),
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

func (r *FloatingIPClaimReconciler) poolToClaims(
	_ context.Context,
	obj ctrlclient.Object,
) []reconcile.Request {
	pool, ok := obj.(*v1alpha2.NutanixFloatingIPPool)
	if !ok {
		return nil
	}
	claims := &v1alpha2.FloatingIPClaimList{}
	if err := r.List(context.Background(), claims, ctrlclient.InNamespace(pool.Namespace)); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for i := range claims.Items {
		claim := &claims.Items[i]
		if claim.Spec.PoolRef.Name == pool.Name {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: ctrlclient.ObjectKeyFromObject(claim),
			})
		}
	}
	return reqs
}

func (r *FloatingIPClaimReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (_ ctrl.Result, reterr error) {
	log := log.FromContext(ctx)

	claim := &v1alpha2.FloatingIPClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	paused, err := isObjectOrClusterPaused(ctx, r.Client, claim, claim.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if paused {
		log.Info("FloatingIPClaim is paused, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(claim, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, claim); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if hasDeleteForMove(claim) {
		controllerutil.RemoveFinalizer(claim, v1alpha2.ReleaseFloatingIPFinalizer)
		return ctrl.Result{}, nil
	}

	if !claim.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, claim)
	}

	if controllerutil.AddFinalizer(claim, v1alpha2.ReleaseFloatingIPFinalizer) {
		return ctrl.Result{}, nil
	}

	return r.reconcileNormal(ctx, claim)
}

func (r *FloatingIPClaimReconciler) reconcileNormal(
	ctx context.Context,
	claim *v1alpha2.FloatingIPClaim,
) (ctrl.Result, error) {
	pool := &v1alpha2.NutanixFloatingIPPool{}
	if err := r.Get(ctx, ctrlclient.ObjectKey{
		Namespace: claim.Namespace,
		Name:      claim.Spec.PoolRef.Name,
	}, pool); err != nil {
		setCondition(
			&claim.Status.Conditions,
			v1alpha2.FloatingIPClaimReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPClaimReadyPoolNotReadyReason,
			err.Error(),
		)
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if annotations.HasPaused(pool) {
		return ctrl.Result{}, nil
	}

	floatingIP, err := r.ensureFloatingIPObject(ctx, claim)
	if err != nil {
		setCondition(
			&claim.Status.Conditions,
			v1alpha2.FloatingIPClaimReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPClaimReadyAllocationFailedReason,
			err.Error(),
		)
		return ctrl.Result{}, err
	}

	claim.Status.FloatingIPRef = &v1alpha2.LocalObjectReference{Name: floatingIP.Name}

	if floatingIP.Spec.ExtID != "" {
		return r.reconcileAllocatedFloatingIP(ctx, claim, pool, floatingIP)
	}

	setAnnotation(claim, clusterctlv1.BlockMoveAnnotation, "")
	setAnnotation(floatingIP, clusterctlv1.BlockMoveAnnotation, "")

	result, err := r.allocateFloatingIP(ctx, claim, pool, floatingIP)
	if err != nil {
		setCondition(
			&claim.Status.Conditions,
			v1alpha2.FloatingIPClaimReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPClaimReadyAllocationFailedReason,
			err.Error(),
		)
		return result, err
	}
	if result.Requeue || result.RequeueAfter > 0 {
		setCondition(
			&claim.Status.Conditions,
			v1alpha2.FloatingIPClaimReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPClaimReadyAllocationFailedReason,
			"Waiting for Prism floating IP allocation",
		)
		return result, nil
	}

	setCondition(
		&claim.Status.Conditions,
		v1alpha2.FloatingIPClaimReadyCondition,
		metav1.ConditionTrue,
		"Allocated",
		"Floating IP allocated",
	)
	return ctrl.Result{}, nil
}

func (r *FloatingIPClaimReconciler) ensureFloatingIPObject(
	ctx context.Context,
	claim *v1alpha2.FloatingIPClaim,
) (*v1alpha2.FloatingIP, error) {
	floatingIP := &v1alpha2.FloatingIP{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
		},
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.Client, floatingIP, func() error {
		if err := controllerutil.SetControllerReference(claim, floatingIP, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(floatingIP, v1alpha2.ProtectFloatingIPFinalizer)
		floatingIP.Spec.ClaimRef = v1alpha2.LocalObjectReference{Name: claim.Name}
		floatingIP.Spec.PoolRef = claim.Spec.PoolRef
		if floatingIP.Spec.PoolRef.Kind == "" {
			floatingIP.Spec.PoolRef.Kind = v1alpha2.NutanixFloatingIPPoolKind
		}
		if floatingIP.Spec.PoolRef.APIGroup == "" {
			floatingIP.Spec.PoolRef.APIGroup = v1alpha2.GroupVersion.Group
		}

		if floatingIP.Annotations == nil {
			floatingIP.Annotations = map[string]string{}
		}
		if floatingIP.Annotations[v1alpha2.PrismFloatingIPNameAnnotation] == "" {
			floatingIP.Annotations[v1alpha2.PrismFloatingIPNameAnnotation] = prismFloatingIPName(
				claim,
			)
		}

		if labels := claim.Labels; labels != nil {
			if clusterName, ok := labels[clusterv1.ClusterNameLabel]; ok {
				if floatingIP.Labels == nil {
					floatingIP.Labels = map[string]string{}
				}
				floatingIP.Labels[clusterv1.ClusterNameLabel] = clusterName
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create or patch FloatingIP: %w", err)
	}

	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), floatingIP); err != nil {
		return nil, err
	}
	return floatingIP, nil
}

func prismFloatingIPName(claim *v1alpha2.FloatingIPClaim) string {
	return fmt.Sprintf("caipamx-%s-%s", claim.Namespace, claim.Name)
}

func (r *FloatingIPClaimReconciler) reconcileAllocatedFloatingIP(
	ctx context.Context,
	claim *v1alpha2.FloatingIPClaim,
	pool *v1alpha2.NutanixFloatingIPPool,
	floatingIP *v1alpha2.FloatingIP,
) (ctrl.Result, error) {
	if hasAnnotation(floatingIP, clusterctlv1.BlockMoveAnnotation) ||
		hasAnnotation(claim, clusterctlv1.BlockMoveAnnotation) {
		removeAnnotation(claim, clusterctlv1.BlockMoveAnnotation)
		removeAnnotation(floatingIP, clusterctlv1.BlockMoveAnnotation)
		if err := r.Update(ctx, floatingIP); err != nil {
			return ctrl.Result{}, err
		}
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
		return ctrl.Result{}, err
	}

	_, err = pcClient.FloatingIPs().Get(ctx, floatingIP.Spec.ExtID)
	if isNotFoundError(err) {
		setCondition(
			&claim.Status.Conditions,
			v1alpha2.FloatingIPClaimReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPClaimReadyAllocationFailedReason,
			fmt.Sprintf(
				"floating IP %q no longer exists in Prism; delete the claim to recover",
				floatingIP.Spec.ExtID,
			),
		)
		latest := &v1alpha2.FloatingIP{}
		if getErr := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); getErr == nil {
			setCondition(
				&latest.Status.Conditions,
				v1alpha2.FloatingIPReadyCondition,
				metav1.ConditionFalse,
				v1alpha2.FloatingIPClaimReadyAllocationFailedReason,
				"Floating IP no longer exists in Prism",
			)
			_ = r.Status().Update(ctx, latest)
		}
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	setCondition(
		&claim.Status.Conditions,
		v1alpha2.FloatingIPClaimReadyCondition,
		metav1.ConditionTrue,
		"Allocated",
		"Floating IP allocated",
	)
	return ctrl.Result{}, nil
}

func (r *FloatingIPClaimReconciler) allocateFloatingIP(
	ctx context.Context,
	claim *v1alpha2.FloatingIPClaim,
	pool *v1alpha2.NutanixFloatingIPPool,
	floatingIP *v1alpha2.FloatingIP,
) (ctrl.Result, error) {
	pcClient, err := getPCClientForPrismCentral(
		pool.Spec.PrismCentral,
		pool.Namespace,
		pool,
		r.SecretInformer,
		r.ConfigMapInformer,
		r.PCClientGetter,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	prismName := floatingIP.Annotations[v1alpha2.PrismFloatingIPNameAnnotation]
	if prismName == "" {
		prismName = prismFloatingIPName(claim)
	}

	if floatingIP.Spec.ExtID == "" {
		existing, err := pcClient.FloatingIPs().ListByName(ctx, prismName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(existing) == 1 {
			return r.persistAllocatedFloatingIP(ctx, floatingIP, &existing[0])
		}
		if len(existing) > 1 {
			return ctrl.Result{}, fmt.Errorf("multiple floating IPs found with name %q", prismName)
		}
	}

	if taskID := floatingIP.Annotations[v1alpha2.NutanixTaskIDAnnotation]; taskID != "" {
		allocated, err := pcClient.FloatingIPs().WaitForCreate(ctx, taskID)
		if err != nil {
			return ctrl.Result{RequeueAfter: floatingIPClaimRequeue}, nil
		}
		return r.persistAllocatedFloatingIP(ctx, floatingIP, allocated)
	}

	subnet, err := pcClient.Networking().GetSubnet(
		ctx,
		pool.Spec.Subnet.String(),
		pcclient.GetSubnetOpts{Cluster: resourceIdentifierString(pool.Spec.Cluster)},
	)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to resolve external subnet: %w", err)
	}

	taskID, err := pcClient.FloatingIPs().CreateAsync(ctx, pcclient.CreateFloatingIPOpts{
		Name:                prismName,
		ExternalSubnetExtID: subnet.ExtID().String(),
		ProjectExtID:        resourceIdentifierString(pool.Spec.Project),
		Address:             claim.Spec.Address,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	setAnnotation(floatingIP, v1alpha2.NutanixTaskIDAnnotation, taskID)
	setAnnotation(floatingIP, clusterctlv1.BlockMoveAnnotation, "")
	if err := r.Update(ctx, floatingIP); err != nil {
		return ctrl.Result{}, err
	}

	allocated, err := pcClient.FloatingIPs().WaitForCreate(ctx, taskID)
	if err != nil {
		return ctrl.Result{RequeueAfter: floatingIPClaimRequeue}, nil
	}
	return r.persistAllocatedFloatingIP(ctx, floatingIP, allocated)
}

func (r *FloatingIPClaimReconciler) persistAllocatedFloatingIP(
	ctx context.Context,
	floatingIP *v1alpha2.FloatingIP,
	allocated *pcclient.FloatingIP,
) (ctrl.Result, error) {
	floatingIP.Spec.ExtID = allocated.ExtID
	floatingIP.Spec.Address = allocated.Address
	if allocated.ExternalSubnetExtID != "" {
		subnet := v1alpha2.NewNutanixResourceIdentifierUUID(allocated.ExternalSubnetExtID)
		floatingIP.Spec.ExternalSubnet = &subnet
	}
	removeAnnotation(floatingIP, v1alpha2.NutanixTaskIDAnnotation)
	removeAnnotation(floatingIP, clusterctlv1.BlockMoveAnnotation)
	if err := r.Update(ctx, floatingIP); err != nil {
		if !apierrors.IsConflict(err) {
			return ctrl.Result{}, err
		}
		latest := &v1alpha2.FloatingIP{}
		if getErr := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); getErr != nil {
			return ctrl.Result{}, getErr
		}
		latest.Spec.ExtID = allocated.ExtID
		latest.Spec.Address = allocated.Address
		if allocated.ExternalSubnetExtID != "" {
			subnet := v1alpha2.NewNutanixResourceIdentifierUUID(allocated.ExternalSubnetExtID)
			latest.Spec.ExternalSubnet = &subnet
		}
		removeAnnotation(latest, v1alpha2.NutanixTaskIDAnnotation)
		removeAnnotation(latest, clusterctlv1.BlockMoveAnnotation)
		if err := r.Update(ctx, latest); err != nil {
			return ctrl.Result{}, err
		}
		*floatingIP = *latest
	}

	latest := &v1alpha2.FloatingIP{}
	if err := r.Get(ctx, ctrlclient.ObjectKeyFromObject(floatingIP), latest); err != nil {
		return ctrl.Result{}, err
	}
	setCondition(
		&latest.Status.Conditions,
		v1alpha2.FloatingIPReadyCondition,
		metav1.ConditionTrue,
		"Allocated",
		"Floating IP allocated in Prism",
	)
	latest.Status.Association = associationStatusFromClient(allocated)
	if err := r.Status().Update(ctx, latest); err != nil {
		return ctrl.Result{}, err
	}
	*floatingIP = *latest
	return ctrl.Result{}, nil
}

func associationStatusFromClient(
	allocated *pcclient.FloatingIP,
) *v1alpha2.FloatingIPAssociationStatus {
	if allocated == nil {
		return nil
	}
	status := &v1alpha2.FloatingIPAssociationStatus{
		Type: v1alpha2.FloatingIPAssociationNotAssociated,
	}
	switch allocated.AssociationType {
	case pcclient.FloatingIPAssociationPrivateIP:
		status.Type = v1alpha2.FloatingIPAssociationPrivateIP
		status.PrivateIP = &v1alpha2.FloatingIPPrivateIPAssociation{
			Address: allocated.PrivateIPAddress,
			VPC:     v1alpha2.NewNutanixResourceIdentifierUUID(allocated.PrivateIPVPCExtID),
		}
	case pcclient.FloatingIPAssociationVMNic:
		status.Type = v1alpha2.FloatingIPAssociationVMNic
		vmNic := v1alpha2.NewNutanixResourceIdentifierUUID(allocated.VMNicExtID)
		status.VMNic = &vmNic
	}
	return status
}

func (r *FloatingIPClaimReconciler) reconcileDelete(
	ctx context.Context,
	claim *v1alpha2.FloatingIPClaim,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	floatingIP := &v1alpha2.FloatingIP{}
	err := r.Get(
		ctx,
		ctrlclient.ObjectKey{Namespace: claim.Namespace, Name: claim.Name},
		floatingIP,
	)
	if apierrors.IsNotFound(err) {
		controllerutil.RemoveFinalizer(claim, v1alpha2.ReleaseFloatingIPFinalizer)
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	if controllerutil.ContainsFinalizer(floatingIP, v1alpha2.BoundFloatingIPFinalizer) ||
		floatingIP.Status.Reservation != nil {
		setCondition(
			&claim.Status.Conditions,
			v1alpha2.FloatingIPClaimReadyCondition,
			metav1.ConditionFalse,
			v1alpha2.FloatingIPClaimReadyWaitingForUnbindReason,
			"Waiting for FloatingIPBinding to release the floating IP",
		)
		return ctrl.Result{RequeueAfter: floatingIPClaimRequeue}, nil
	}

	if floatingIP.Spec.ExtID != "" {
		pool := &v1alpha2.NutanixFloatingIPPool{}
		if err := r.Get(ctx, ctrlclient.ObjectKey{
			Namespace: claim.Namespace,
			Name:      claim.Spec.PoolRef.Name,
		}, pool); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if pool.Name != "" {
			pcClient, err := getPCClientForPrismCentral(
				pool.Spec.PrismCentral,
				pool.Namespace,
				pool,
				r.SecretInformer,
				r.ConfigMapInformer,
				r.PCClientGetter,
			)
			if err != nil {
				return ctrl.Result{}, err
			}

			current, err := pcClient.FloatingIPs().Get(ctx, floatingIP.Spec.ExtID)
			if err == nil {
				if current.AssociationType != pcclient.FloatingIPAssociationNone {
					setCondition(
						&claim.Status.Conditions,
						v1alpha2.FloatingIPClaimReadyCondition,
						metav1.ConditionFalse,
						v1alpha2.FloatingIPClaimReadyWaitingForUnbindReason,
						"Waiting for Prism floating IP to become unassociated",
					)
					return ctrl.Result{RequeueAfter: floatingIPClaimRequeue}, nil
				}
				if err := pcClient.FloatingIPs().Delete(ctx, floatingIP.Spec.ExtID); err != nil {
					return ctrl.Result{}, err
				}
			} else if !isNotFoundError(err) {
				return ctrl.Result{}, err
			}
		}
	}

	if controllerutil.RemoveFinalizer(floatingIP, v1alpha2.ProtectFloatingIPFinalizer) {
		if err := r.Update(ctx, floatingIP); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if err := r.Delete(ctx, floatingIP); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(claim, v1alpha2.ReleaseFloatingIPFinalizer)
	log.Info("Released floating IP claim")
	return ctrl.Result{}, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return apierrors.IsNotFound(err) ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "Not Found") ||
		strings.Contains(msg, "ENTITY_NOT_FOUND") ||
		strings.Contains(msg, "does not exist")
}
