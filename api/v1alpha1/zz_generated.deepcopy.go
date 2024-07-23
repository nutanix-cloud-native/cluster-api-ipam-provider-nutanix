//go:build !ignore_autogenerated

// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LocalSecretRef) DeepCopyInto(out *LocalSecretRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LocalSecretRef.
func (in *LocalSecretRef) DeepCopy() *LocalSecretRef {
	if in == nil {
		return nil
	}
	out := new(LocalSecretRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixIPPool) DeepCopyInto(out *NutanixIPPool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixIPPool.
func (in *NutanixIPPool) DeepCopy() *NutanixIPPool {
	if in == nil {
		return nil
	}
	out := new(NutanixIPPool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NutanixIPPool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixIPPoolList) DeepCopyInto(out *NutanixIPPoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NutanixIPPool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixIPPoolList.
func (in *NutanixIPPoolList) DeepCopy() *NutanixIPPoolList {
	if in == nil {
		return nil
	}
	out := new(NutanixIPPoolList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NutanixIPPoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixIPPoolSpec) DeepCopyInto(out *NutanixIPPoolSpec) {
	*out = *in
	out.PrismCentral = in.PrismCentral
	if in.Cluster != nil {
		in, out := &in.Cluster, &out.Cluster
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixIPPoolSpec.
func (in *NutanixIPPoolSpec) DeepCopy() *NutanixIPPoolSpec {
	if in == nil {
		return nil
	}
	out := new(NutanixIPPoolSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NutanixIPPoolStatus) DeepCopyInto(out *NutanixIPPoolStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NutanixIPPoolStatus.
func (in *NutanixIPPoolStatus) DeepCopy() *NutanixIPPoolStatus {
	if in == nil {
		return nil
	}
	out := new(NutanixIPPoolStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PrismCentral) DeepCopyInto(out *PrismCentral) {
	*out = *in
	out.CredentialsSecretRef = in.CredentialsSecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PrismCentral.
func (in *PrismCentral) DeepCopy() *PrismCentral {
	if in == nil {
		return nil
	}
	out := new(PrismCentral)
	in.DeepCopyInto(out)
	return out
}
