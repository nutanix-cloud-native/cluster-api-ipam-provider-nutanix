// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func hasDeleteForMove(obj metav1.Object) bool {
	return hasAnnotation(obj, clusterctlv1.DeleteForMoveAnnotation)
}

func hasAnnotation(obj metav1.Object, key string) bool {
	anns := obj.GetAnnotations()
	if anns == nil {
		return false
	}
	_, ok := anns[key]
	return ok
}

func setAnnotation(obj metav1.Object, key, value string) {
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[key] = value
	obj.SetAnnotations(anns)
}

func removeAnnotation(obj metav1.Object, key string) {
	anns := obj.GetAnnotations()
	if anns == nil {
		return
	}
	delete(anns, key)
	obj.SetAnnotations(anns)
}

func isObjectOrClusterPaused(
	ctx context.Context,
	c ctrlclient.Client,
	obj ctrlclient.Object,
	clusterName string,
) (bool, error) {
	if annotations.HasPaused(obj) {
		return true, nil
	}

	name := clusterName
	if name == "" {
		if labels := obj.GetLabels(); labels != nil {
			name = labels[clusterv1.ClusterNameLabel]
		}
	}
	if name == "" {
		return false, nil
	}

	cluster, err := util.GetClusterByName(ctx, c, obj.GetNamespace(), name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return annotations.IsPaused(cluster, cluster), nil
}
