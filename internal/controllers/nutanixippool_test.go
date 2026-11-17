// Copyright 2026 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/index"
)

const (
	poolNamespace = "default"
	poolTestName  = "workload-control-plane-endpoint"
)

func poolScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(ipamv1.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	return scheme
}

func testPool(deleting bool, finalizers ...string) *v1alpha1.NutanixIPPool {
	pool := &v1alpha1.NutanixIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolTestName,
			Namespace:  poolNamespace,
			Finalizers: finalizers,
		},
		Spec: v1alpha1.NutanixIPPoolSpec{Subnet: "vlan170"},
	}
	if deleting {
		now := metav1.NewTime(time.Now())
		pool.DeletionTimestamp = &now
	}
	return pool
}

func testClaim(name string) *ipamv1.IPAddressClaim {
	return &ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: poolNamespace},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: ipamv1.IPPoolReference{
				Name:     poolTestName,
				Kind:     v1alpha1.NutanixIPPoolKind,
				APIGroup: v1alpha1.GroupVersion.Group,
			},
		},
	}
}

func poolClient(t *testing.T, objs ...ctrlclient.Object) ctrlclient.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(poolScheme(t)).
		WithObjects(objs...).
		WithIndex(&ipamv1.IPAddressClaim{}, index.IPAddressClaimPoolRefCombinedField,
			func(o ctrlclient.Object) []string {
				claim, ok := o.(*ipamv1.IPAddressClaim)
				NewWithT(t).Expect(ok).To(BeTrue())
				return []string{index.IPPoolRefValue(claim.Spec.PoolRef)}
			}).
		Build()
}

func reconcilePool(t *testing.T, c ctrlclient.Client) {
	t.Helper()
	r := &NutanixIPPoolReconciler{Client: c}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: ctrlclient.ObjectKey{Namespace: poolNamespace, Name: poolTestName},
	})
	NewWithT(t).Expect(err).NotTo(HaveOccurred())
}

func livePool(t *testing.T, c ctrlclient.Client) *v1alpha1.NutanixIPPool {
	t.Helper()
	pool := &v1alpha1.NutanixIPPool{}
	err := c.Get(context.Background(),
		ctrlclient.ObjectKey{Namespace: poolNamespace, Name: poolTestName}, pool)
	if err != nil {
		return nil
	}
	return pool
}

func TestNutanixIPPoolReconciler(t *testing.T) {
	t.Parallel()

	t.Run("a live pool is protected", func(t *testing.T) {
		t.Parallel()
		c := poolClient(t, testPool(false))
		reconcilePool(t, c)
		NewWithT(t).Expect(livePool(t, c).Finalizers).To(ContainElement(ProtectPoolFinalizer))
	})

	t.Run("protecting twice does not duplicate the finalizer", func(t *testing.T) {
		t.Parallel()
		c := poolClient(t, testPool(false))
		reconcilePool(t, c)
		reconcilePool(t, c)
		NewWithT(t).Expect(livePool(t, c).Finalizers).To(HaveLen(1))
	})

	// The regression. Deleting a cluster garbage-collects the pool and the
	// claims together; if the pool goes first the address can never be
	// released, because the Prism Central and subnet are only on the pool.
	t.Run("a deleting pool is held while a claim still references it", func(t *testing.T) {
		t.Parallel()
		c := poolClient(t,
			testPool(true, ProtectPoolFinalizer),
			testClaim(poolTestName),
		)
		reconcilePool(t, c)
		g := NewWithT(t)
		pool := livePool(t, c)
		g.Expect(pool).NotTo(BeNil(), "the pool must still exist")
		g.Expect(pool.Finalizers).To(ContainElement(ProtectPoolFinalizer),
			"releasing the address needs the pool, so it must outlive the claim")
	})

	t.Run("a deleting pool is let go once no claim references it", func(t *testing.T) {
		t.Parallel()
		c := poolClient(t, testPool(true, ProtectPoolFinalizer))
		reconcilePool(t, c)
		if pool := livePool(t, c); pool != nil {
			NewWithT(t).Expect(pool.Finalizers).NotTo(ContainElement(ProtectPoolFinalizer))
		}
	})

	t.Run("a claim for a different pool does not hold this one", func(t *testing.T) {
		t.Parallel()
		other := testClaim("someone-else")
		other.Spec.PoolRef.Name = "another-pool"
		c := poolClient(t, testPool(true, ProtectPoolFinalizer), other)
		reconcilePool(t, c)
		if pool := livePool(t, c); pool != nil {
			NewWithT(t).Expect(pool.Finalizers).NotTo(ContainElement(ProtectPoolFinalizer),
				"only claims against this pool may hold it")
		}
	})

	t.Run("a missing pool is not an error", func(t *testing.T) {
		t.Parallel()
		reconcilePool(t, poolClient(t))
	})
}
