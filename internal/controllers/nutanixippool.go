// Copyright 2026 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"

	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
)

// ProtectPoolFinalizer keeps a NutanixIPPool alive while claims still reference
// it.
//
// Releasing an address needs the pool: it carries the Prism Central endpoint,
// the credentials and the subnet. A pool created for a cluster is owner
// referenced to that cluster, so deleting the cluster makes Kubernetes garbage
// collect the pool and the claims together, in no particular order. When the
// pool loses that race the address can never be released - there is nothing
// left to say which Prism Central or subnet it came from - and the reservation
// is stranded for good.
//
// Measured on a live cluster: deleting one left 10.22.199.174 reserved with the
// claim wedged on its own finalizer and the pool already gone.
const ProtectPoolFinalizer = "ipam.cluster.x-k8s.io/ProtectPool"

// Setting and clearing the finalizer is an update of the pool itself, which the
// claim reconciler never needed.
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=nutanixippools,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch

// NutanixIPPoolReconciler holds a pool open until the addresses taken from it
// have been released.
type NutanixIPPoolReconciler struct {
	Client ctrlclient.Client
}

// SetupWithManager sets up the controller with the Manager.
func (r *NutanixIPPoolReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NutanixIPPool{}).
		Named("nutanixippool").
		Complete(r)
}

func (r *NutanixIPPoolReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pool := &v1alpha1.NutanixIPPool{}
	if err := r.Client.Get(ctx, req.NamespacedName, pool); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
	}

	if pool.DeletionTimestamp.IsZero() {
		// Take the finalizer before any claim can be made against the pool, so
		// there is no window in which a pool holds addresses unprotected.
		if controllerutil.AddFinalizer(pool, ProtectPoolFinalizer) {
			if err := r.Client.Update(ctx, pool); err != nil {
				return ctrl.Result{}, fmt.Errorf("add pool finalizer: %w", err)
			}
		}
		return ctrl.Result{}, nil
	}

	claims, err := r.claimsReferencing(ctx, pool)
	if err != nil {
		return ctrl.Result{}, err
	}
	if claims > 0 {
		// Hold the pool. The claim reconciler is releasing those addresses and
		// each release deletes a claim, which wakes this controller again, so
		// there is no need to poll.
		logger.Info("Pool is deleting but addresses are still allocated from it; holding it open",
			"claims", claims)
		return ctrl.Result{}, nil
	}

	if controllerutil.RemoveFinalizer(pool, ProtectPoolFinalizer) {
		if err := r.Client.Update(ctx, pool); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove pool finalizer: %w", err)
		}
		logger.Info("All addresses released; pool may be deleted")
	}
	return ctrl.Result{}, nil
}

// claimsReferencing counts the claims still pointing at pool.
func (r *NutanixIPPoolReconciler) claimsReferencing(
	ctx context.Context,
	pool *v1alpha1.NutanixIPPool,
) (int, error) {
	claims := &ipamv1.IPAddressClaimList{}
	if err := r.Client.List(ctx, claims,
		ctrlclient.MatchingFields{
			index.IPAddressClaimPoolRefCombinedField: index.IPPoolRefValue(ipamv1.IPPoolReference{
				Name:     pool.GetName(),
				Kind:     v1alpha1.NutanixIPPoolKind,
				APIGroup: v1alpha1.GroupVersion.Group,
			}),
		},
		ctrlclient.InNamespace(pool.GetNamespace()),
	); err != nil {
		return 0, fmt.Errorf("list IPAddressClaims for pool %s/%s: %w",
			pool.GetNamespace(), pool.GetName(), err)
	}
	return len(claims.Items), nil
}
