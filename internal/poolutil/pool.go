// Copyright 2024 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package poolutil implements utility functions to manage a pool of IP addresses.
package poolutil

import (
	"fmt"
	"math/big"

	"go4.org/netipx"
)

// IPSetCount returns the number of IPs contained in the given IPSet.
// This function returns type int64, which is smaller than
// the possible size of a range, which could be 2^128 IPs.
// When an IPSet's count is would be larger than max int64, an error
// is returned.
func IPSetCount(ipSet *netipx.IPSet) (int64, error) {
	if ipSet == nil {
		return 0, nil
	}

	total := big.NewInt(0)
	for _, iprange := range ipSet.Ranges() {
		total.Add(
			total,
			big.NewInt(0).Sub(
				big.NewInt(0).SetBytes(iprange.To().AsSlice()),
				big.NewInt(0).SetBytes(iprange.From().AsSlice()),
			),
		)
		// Subtracting To and From misses that one of those is a valid IP
		total.Add(total, big.NewInt(1))
	}

	// If total is greater than Int64, Int64() will return 0.
	// We want to display MaxInt64 if the value overflows what int64 can contain.
	if total.IsInt64() {
		return total.Int64(), nil
	}

	return 0, fmt.Errorf("IPSet count is too large to fit in an int64")
}
