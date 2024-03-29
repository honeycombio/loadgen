// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE-go file.

package rand

func Int31nForTest(r *Rand, n int32) int32 {
	return r.Int31n(n)
}

func GetNormalDistributionParameters() (float64, [256]uint64, [256]float64, [256]float64) {
	return rn, kn, wn, fn
}

func GetExponentialDistributionParameters() (float64, [256]uint64, [256]float64, [256]float64) {
	return re, ke, we, fe
}

var ShuffleSliceGeneric func(*Rand, []int)
