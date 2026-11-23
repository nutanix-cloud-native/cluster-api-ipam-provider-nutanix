// Copyright 2026 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha1"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
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

// The reservation failures that must not be retried hard: an exhausted subnet
// will not refill in the controller's 500ms backoff floor, and retrying into
// Prism Central's rate limit is how one stuck claim starts failing reservations
// for every other cluster sharing that Prism Central.
func TestClassifyReserveError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantReason string
		wantWait   bool
	}{
		{
			// Verbatim from a live Prism Central with a full pool.
			name: "an exhausted subnet pool waits",
			err: errors.New(`task failed: {"legacyErrorMessage":"Failed to issue ` +
				`NetworkReserveIp RPC: kAddressPoolExhausted: Could not get free IP address ` +
				`for network d4ade013-d1d1-4eac-9ab8-cdcc91ad4cec."}`),
			wantReason: ReasonPoolExhausted,
			wantWait:   true,
		},
		{
			// Verbatim from the same run, one second later.
			name: "a rate limited Prism Central waits",
			err: errors.New(`{"message":"Failed to perform the operation as rate limit ` +
				`set by the API has been exceeded.","code":"PLAT-10003",` +
				`"errorGroup":"RATE_LIMIT_EXCEEDED"}`),
			wantReason: ReasonRateLimited,
			wantWait:   true,
		},
		{
			name:       "the rate limit is recognised by its error group too",
			err:        errors.New("something: RATE_LIMIT_EXCEEDED"),
			wantReason: ReasonRateLimited,
			wantWait:   true,
		},
		{
			// Anything else may be transient and worth retrying promptly.
			name:       "an unknown failure is retried as an error",
			err:        errors.New("connection reset by peer"),
			wantReason: ReasonReserveFailed,
			wantWait:   false,
		},
		{
			name:       "a missing subnet is not something waiting fixes, so it surfaces",
			err:        errors.New("NETWORKING-10060 Existing subnet is not found"),
			wantReason: ReasonReserveFailed,
			wantWait:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			reason, wait := classifyReserveError(tt.err)
			g.Expect(reason).To(Equal(tt.wantReason))
			g.Expect(wait).To(Equal(tt.wantWait))
		})
	}
}

// A failure must leave a reason on the claim: before this, an exhausted pool was
// visible only in this controller's log, so a cluster waiting on the address
// hung with nothing explaining why.
func TestReservationFailedRecordsAReason(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	h := &IPAddressClaimHandler{claim: &ipamv1.IPAddressClaim{}}
	res, err := h.reservationFailed(context.Background(),
		errors.New("task failed: kAddressPoolExhausted: Could not get free IP address"))

	g.Expect(err).NotTo(HaveOccurred(), "an exhausted pool must not be retried hard")
	g.Expect(res).NotTo(BeNil())
	g.Expect(res.RequeueAfter).To(Equal(retryAfterUnavailable))

	cond := meta.FindStatusCondition(h.claim.Status.Conditions, ReadyCondition)
	g.Expect(cond).NotTo(BeNil(), "the claim must say why it has no address")
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(ReasonPoolExhausted))
	g.Expect(cond.Message).To(ContainSubstring("kAddressPoolExhausted"))
}

func TestReservationFailedSurfacesUnknownErrors(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	h := &IPAddressClaimHandler{claim: &ipamv1.IPAddressClaim{}}
	_, err := h.reservationFailed(context.Background(), errors.New("connection reset by peer"))

	g.Expect(err).To(HaveOccurred(), "an unclassified failure must still be reported")
	cond := meta.FindStatusCondition(h.claim.Status.Conditions, ReadyCondition)
	g.Expect(cond).NotTo(BeNil())
	g.Expect(cond.Reason).To(Equal(ReasonReserveFailed))
}

// The regression: a pool being deleted waits for its claims, and nothing
// brought it back here when the last claim went, so it sat holding its
// finalizer forever.
func TestClaimToPoolMapping(t *testing.T) {
	t.Parallel()

	t.Run("a claim maps to the pool it draws from", func(t *testing.T) {
		t.Parallel()
		reqs := claimToPool(context.Background(), testClaim("c"))
		NewWithT(t).Expect(reqs).To(Equal([]reconcile.Request{{
			NamespacedName: types.NamespacedName{Namespace: poolNamespace, Name: poolTestName},
		}}))
	})

	t.Run("a claim against another kind of pool is ignored", func(t *testing.T) {
		t.Parallel()
		claim := testClaim("c")
		claim.Spec.PoolRef.Kind = "InClusterIPPool"
		NewWithT(t).Expect(claimToPool(context.Background(), claim)).To(BeEmpty())
	})

	t.Run("a claim from another API group is ignored", func(t *testing.T) {
		t.Parallel()
		claim := testClaim("c")
		claim.Spec.PoolRef.APIGroup = "example.com"
		NewWithT(t).Expect(claimToPool(context.Background(), claim)).To(BeEmpty())
	})

	t.Run("a non-claim object is ignored", func(t *testing.T) {
		t.Parallel()
		NewWithT(t).Expect(claimToPool(context.Background(), testPool(false))).To(BeEmpty())
	})
}

// poolWithCredentials reaches the Prism Central credentials lookup in
// getClient, which is where a deleted cluster's missing Secret surfaces.
func poolWithCredentials() *v1alpha1.NutanixIPPool {
	pool := testPool(false)
	pool.Spec.PrismCentral = v1alpha1.PrismCentral{
		Address:              "pc.example.com",
		Port:                 9440,
		Insecure:             true,
		CredentialsSecretRef: v1alpha1.LocalSecretRef{Name: "workload-pc-credentials"},
	}
	return pool
}

// emptySecretInformer has no Secrets, so the credentials lookup returns
// NotFound - exactly what a deleted cluster leaves behind.
func emptySecretInformer() coreinformers.SecretInformer {
	return informers.NewSharedInformerFactory(k8sfake.NewClientset(), 0).Core().V1().Secrets()
}

// secretInformerWith holds the credentials Secret, so the lookup succeeds and
// the failure under test comes from building the client instead.
func secretInformerWith(secret *corev1.Secret) coreinformers.SecretInformer {
	si := informers.NewSharedInformerFactory(k8sfake.NewClientset(secret), 0).Core().V1().Secrets()
	utilruntime.Must(si.Informer().GetIndexer().Add(secret))
	return si
}

func transientSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "workload-pc-credentials", Namespace: poolNamespace},
		Data: map[string][]byte{
			"credentials": []byte(
				`[{"type":"basic_auth","data":{"prismCentral":{"username":"u","password":"p"}}}]`,
			),
		},
	}
}

// ReleaseAddress is called even when FetchPool failed, which the ClaimHandler
// contract spells out, so the pool can be nil. It must fail loudly and keep the
// finalizer rather than dereference nil and drop a reserved address.
func TestReleaseAddressWithoutAPoolFails(t *testing.T) {
	g := NewWithT(t)

	claim := testClaim("claim-no-pool")
	claim.Status.AddressRef = ipamv1.IPAddressReference{Name: "addr-1"}

	h := &IPAddressClaimHandler{
		client: poolClient(t, claim),
		claim:  claim,
		pool:   nil,
	}

	res, err := h.ReleaseAddress(context.Background())
	g.Expect(res).To(BeNil())
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("is gone"))
}

// A deleted cluster takes its Prism Central credentials Secret with it, so a
// client can never be built again and retrying only wedges the claim behind a
// finalizer forever. A NotFound must therefore be terminal: record why, and let
// the claim go. CAREN's BeforeClusterDelete sweep is the designed backstop.
func TestReleaseAddressWithGoneCredentialsIsTerminal(t *testing.T) {
	g := NewWithT(t)

	pool := poolWithCredentials()
	claim := testClaim("claim-creds-gone")
	claim.Status.AddressRef = ipamv1.IPAddressReference{Name: "addr-1"}
	address := &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "addr-1", Namespace: poolNamespace},
	}

	h := &IPAddressClaimHandler{
		client:         poolClient(t, pool, claim, address),
		claim:          claim,
		pool:           pool,
		secretInformer: emptySecretInformer(),
	}

	res, err := h.ReleaseAddress(context.Background())
	g.Expect(err).NotTo(HaveOccurred(), "a retry can never succeed, so it must not be an error")
	g.Expect(res).To(BeNil())

	cond := meta.FindStatusCondition(claim.Status.Conditions, ReadyCondition)
	g.Expect(cond).NotTo(BeNil(), "the reason must be recorded on the claim")
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(ReasonCredentialsGone))
	g.Expect(cond.Message).To(ContainSubstring("may remain reserved"))
}

// Any other client failure can recover, so it must stay an error and retry.
func TestReleaseAddressWithATransientClientFailureRetries(t *testing.T) {
	g := NewWithT(t)

	pool := poolWithCredentials()
	claim := testClaim("claim-transient")
	claim.Status.AddressRef = ipamv1.IPAddressReference{Name: "addr-1"}
	address := &ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "addr-1", Namespace: poolNamespace},
	}

	h := &IPAddressClaimHandler{
		client:         poolClient(t, pool, claim, address),
		claim:          claim,
		pool:           pool,
		secretInformer: secretInformerWith(transientSecret()),
		pcClientGetter: func(pcclient.CachedClientParams) (pcclient.Client, error) {
			return nil, errors.New("dial tcp: i/o timeout")
		},
	}

	_, err := h.ReleaseAddress(context.Background())
	g.Expect(err).To(HaveOccurred())
	g.Expect(meta.FindStatusCondition(claim.Status.Conditions, ReasonCredentialsGone)).To(BeNil())
}
