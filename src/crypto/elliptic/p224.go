// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package elliptic

import (
	"crypto/elliptic/internal/nistec"
	"crypto/rand"
	"math/big"
)

<<<<<<< HEAD
// p224Curve is a Curve implementation based on nistec.P224Point.
=======
var p224 p224Curve

type p224Curve struct {
	*CurveParams
	gx, gy, b p224FieldElement
}

func initP224() {
	// See FIPS 186-3, section D.2.2
	p224.CurveParams = &CurveParams{Name: "P-224"}
	p224.P, _ = new(big.Int).SetString("26959946667150639794667015087019630673557916260026308143510066298881", 10)
	p224.N, _ = new(big.Int).SetString("26959946667150639794667015087019625940457807714424391721682722368061", 10)
	p224.B, _ = new(big.Int).SetString("b4050a850c04b3abf54132565044b0b7d7bfd8ba270b39432355ffb4", 16)
	p224.Gx, _ = new(big.Int).SetString("b70e0cbd6bb4bf7f321390b94a03c1d356c21122343280d6115c1d21", 16)
	p224.Gy, _ = new(big.Int).SetString("bd376388b5f723fb4c22dfe6cd4375a05a07476444d5819985007e34", 16)
	p224.BitSize = 224

	p224FromBig(&p224.gx, p224.Gx)
	p224FromBig(&p224.gy, p224.Gy)
	p224FromBig(&p224.b, p224.B)
}

// P224 returns a Curve which implements P-224 (see FIPS 186-3, section D.2.2).
//
// The cryptographic operations are implemented using constant-time algorithms.
func P224() Curve {
	initonce.Do(initAll)
	return p224
}

func (curve p224Curve) Params() *CurveParams {
	return curve.CurveParams
}

func (curve p224Curve) IsOnCurve(bigX, bigY *big.Int) bool {
	if bigX.Sign() < 0 || bigX.Cmp(curve.P) >= 0 ||
		bigY.Sign() < 0 || bigY.Cmp(curve.P) >= 0 {
		return false
	}

	var x, y p224FieldElement
	p224FromBig(&x, bigX)
	p224FromBig(&y, bigY)

	// y² = x³ - 3x + b
	var tmp p224LargeFieldElement
	var x3 p224FieldElement
	p224Square(&x3, &x, &tmp)
	p224Mul(&x3, &x3, &x, &tmp)

	for i := 0; i < 8; i++ {
		x[i] *= 3
	}
	p224Sub(&x3, &x3, &x)
	p224Reduce(&x3)
	p224Add(&x3, &x3, &curve.b)
	p224Contract(&x3, &x3)

	p224Square(&y, &y, &tmp)
	p224Contract(&y, &y)

	for i := 0; i < 8; i++ {
		if y[i] != x3[i] {
			return false
		}
	}
	return true
}

func (p224Curve) Add(bigX1, bigY1, bigX2, bigY2 *big.Int) (x, y *big.Int) {
	var x1, y1, z1, x2, y2, z2, x3, y3, z3 p224FieldElement

	p224FromBig(&x1, bigX1)
	p224FromBig(&y1, bigY1)
	if bigX1.Sign() != 0 || bigY1.Sign() != 0 {
		z1[0] = 1
	}
	p224FromBig(&x2, bigX2)
	p224FromBig(&y2, bigY2)
	if bigX2.Sign() != 0 || bigY2.Sign() != 0 {
		z2[0] = 1
	}

	p224AddJacobian(&x3, &y3, &z3, &x1, &y1, &z1, &x2, &y2, &z2)
	return p224ToAffine(&x3, &y3, &z3)
}

func (p224Curve) Double(bigX1, bigY1 *big.Int) (x, y *big.Int) {
	var x1, y1, z1, x2, y2, z2 p224FieldElement

	p224FromBig(&x1, bigX1)
	p224FromBig(&y1, bigY1)
	z1[0] = 1

	p224DoubleJacobian(&x2, &y2, &z2, &x1, &y1, &z1)
	return p224ToAffine(&x2, &y2, &z2)
}

func (p224Curve) ScalarMult(bigX1, bigY1 *big.Int, scalar []byte) (x, y *big.Int) {
	var x1, y1, z1, x2, y2, z2 p224FieldElement

	p224FromBig(&x1, bigX1)
	p224FromBig(&y1, bigY1)
	z1[0] = 1

	p224ScalarMult(&x2, &y2, &z2, &x1, &y1, &z1, scalar)
	return p224ToAffine(&x2, &y2, &z2)
}

func (curve p224Curve) ScalarBaseMult(scalar []byte) (x, y *big.Int) {
	var z1, x2, y2, z2 p224FieldElement

	z1[0] = 1
	p224ScalarMult(&x2, &y2, &z2, &curve.gx, &curve.gy, &z1, scalar)
	return p224ToAffine(&x2, &y2, &z2)
}

// Field element functions.
//
// The field that we're dealing with is ℤ/pℤ where p = 2**224 - 2**96 + 1.
//
// Field elements are represented by a FieldElement, which is a typedef to an
// array of 8 uint32's. The value of a FieldElement, a, is:
//   a[0] + 2**28·a[1] + 2**56·a[1] + ... + 2**196·a[7]
>>>>>>> 346b18ee9d15410ab08dd583787c64dbed0666d2
//
// It's a wrapper that exposes the big.Int-based Curve interface and encodes the
// legacy idiosyncrasies it requires, such as invalid and infinity point
// handling.
//
// To interact with the nistec package, points are encoded into and decoded from
// properly formatted byte slices. All big.Int use is limited to this package.
// Encoding and decoding is 1/1000th of the runtime of a scalar multiplication,
// so the overhead is acceptable.
type p224Curve struct {
	params *CurveParams
}

var p224 p224Curve
var _ Curve = p224

func initP224() {
	p224.params = &CurveParams{
		Name:    "P-224",
		BitSize: 224,
		// FIPS 186-4, section D.1.2.2
		P:  bigFromDecimal("26959946667150639794667015087019630673557916260026308143510066298881"),
		N:  bigFromDecimal("26959946667150639794667015087019625940457807714424391721682722368061"),
		B:  bigFromHex("b4050a850c04b3abf54132565044b0b7d7bfd8ba270b39432355ffb4"),
		Gx: bigFromHex("b70e0cbd6bb4bf7f321390b94a03c1d356c21122343280d6115c1d21"),
		Gy: bigFromHex("bd376388b5f723fb4c22dfe6cd4375a05a07476444d5819985007e34"),
	}
}

func (curve p224Curve) Params() *CurveParams {
	return curve.params
}

func (curve p224Curve) IsOnCurve(x, y *big.Int) bool {
	// IsOnCurve is documented to reject (0, 0), the conventional point at
	// infinity, which however is accepted by p224PointFromAffine.
	if x.Sign() == 0 && y.Sign() == 0 {
		return false
	}
	_, ok := p224PointFromAffine(x, y)
	return ok
}

func p224PointFromAffine(x, y *big.Int) (p *nistec.P224Point, ok bool) {
	// (0, 0) is by convention the point at infinity, which can't be represented
	// in affine coordinates. Marshal incorrectly encodes it as an uncompressed
	// point, which SetBytes would correctly reject. See Issue 37294.
	if x.Sign() == 0 && y.Sign() == 0 {
		return nistec.NewP224Point(), true
	}
	if x.Sign() < 0 || y.Sign() < 0 {
		return nil, false
	}
	if x.BitLen() > 224 || y.BitLen() > 224 {
		return nil, false
	}
	p, err := nistec.NewP224Point().SetBytes(Marshal(P224(), x, y))
	if err != nil {
		return nil, false
	}
	return p, true
}

func p224PointToAffine(p *nistec.P224Point) (x, y *big.Int) {
	out := p.Bytes()
	if len(out) == 1 && out[0] == 0 {
		// This is the correct encoding of the point at infinity, which
		// Unmarshal does not support. See Issue 37294.
		return new(big.Int), new(big.Int)
	}
	x, y = Unmarshal(P224(), out)
	if x == nil {
		panic("crypto/elliptic: internal error: Unmarshal rejected a valid point encoding")
	}
	return x, y
}

// p224RandomPoint returns a random point on the curve. It's used when Add,
// Double, or ScalarMult are fed a point not on the curve, which is undefined
// behavior. Originally, we used to do the math on it anyway (which allows
// invalid curve attacks) and relied on the caller and Unmarshal to avoid this
// happening in the first place. Now, we just can't construct a nistec.P224Point
// for an invalid pair of coordinates, because that API is safer. If we panic,
// we risk introducing a DoS. If we return nil, we risk a panic. If we return
// the input, ecdsa.Verify might fail open. The safest course seems to be to
// return a valid, random point, which hopefully won't help the attacker.
func p224RandomPoint() (x, y *big.Int) {
	_, x, y, err := GenerateKey(P224(), rand.Reader)
	if err != nil {
		panic("crypto/elliptic: failed to generate random point")
	}
	return x, y
}

func (p224Curve) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	p1, ok := p224PointFromAffine(x1, y1)
	if !ok {
		return p224RandomPoint()
	}
	p2, ok := p224PointFromAffine(x2, y2)
	if !ok {
		return p224RandomPoint()
	}
	return p224PointToAffine(p1.Add(p1, p2))
}

func (p224Curve) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	p, ok := p224PointFromAffine(x1, y1)
	if !ok {
		return p224RandomPoint()
	}
	return p224PointToAffine(p.Double(p))
}

func (p224Curve) ScalarMult(Bx, By *big.Int, scalar []byte) (*big.Int, *big.Int) {
	p, ok := p224PointFromAffine(Bx, By)
	if !ok {
		return p224RandomPoint()
	}
	return p224PointToAffine(p.ScalarMult(p, scalar))
}

func (p224Curve) ScalarBaseMult(scalar []byte) (*big.Int, *big.Int) {
	p := nistec.NewP224Generator()
	return p224PointToAffine(p.ScalarMult(p, scalar))
}
