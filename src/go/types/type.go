// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

// A Type represents a type of Go.
// All types implement the Type interface.
type Type interface {
	// Underlying returns the underlying type of a type
	// w/o following forwarding chains. Only used by
	// client packages.
	Underlying() Type

	// String returns a string representation of a type.
	String() string
}

// under returns the true expanded underlying type.
// If it doesn't exist, the result is Typ[Invalid].
// under must only be called when a type is known
// to be fully set up.
func under(t Type) Type {
	if t, _ := t.(*Named); t != nil {
		return t.under()
	}
	return t.Underlying()
}

// If t is not a type parameter, coreType returns the underlying type.
// If t is a type parameter, coreType returns the single underlying
// type of all types in its type set if it exists, or nil otherwise. If the
// type set contains only unrestricted and restricted channel types (with
// identical element types), the single underlying type is the restricted
// channel type if the restrictions are always the same, or nil otherwise.
func coreType(t Type) Type {
	tpar, _ := t.(*TypeParam)
	if tpar == nil {
		return under(t)
	}

	var su Type
	if tpar.underIs(func(u Type) bool {
		if u == nil {
			return false
		}
		if su != nil {
			u = match(su, u)
			if u == nil {
				return false
			}
		}
		// su == nil || match(su, u) != nil
		su = u
		return true
	}) {
		return su
	}
	return nil
}

// coreString is like coreType but also considers []byte
// and strings as identical. In this case, if successful and we saw
// a string, the result is of type (possibly untyped) string.
func coreString(t Type) Type {
	tpar, _ := t.(*TypeParam)
	if tpar == nil {
		return under(t) // string or untyped string
	}

	var su Type
	hasString := false
	if tpar.underIs(func(u Type) bool {
		if u == nil {
			return false
		}
		if isString(u) {
			u = NewSlice(universeByte)
			hasString = true
		}
		if su != nil {
			u = match(su, u)
			if u == nil {
				return false
			}
		}
		// su == nil || match(su, u) != nil
		su = u
		return true
	}) {
		if hasString {
			return Typ[String]
		}
		return su
	}
	return nil
}

// If x and y are identical, match returns x.
// If x and y are identical channels but for their direction
// and one of them is unrestricted, match returns the channel
// with the restricted direction.
// In all other cases, match returns nil.
func match(x, y Type) Type {
	// Common case: we don't have channels.
	if Identical(x, y) {
		return x
	}

	// We may have channels that differ in direction only.
	if x, _ := x.(*Chan); x != nil {
		if y, _ := y.(*Chan); y != nil && Identical(x.elem, y.elem) {
			// We have channels that differ in direction only.
			// If there's an unrestricted channel, select the restricted one.
			switch {
			case x.dir == SendRecv:
				return y
			case y.dir == SendRecv:
				return x
			}
<<<<<<< HEAD
=======
			typ.check = nil
		})
	}
	return typ
}

// Obj returns the type name for the named type t.
func (t *Named) Obj() *TypeName { return t.obj }

// TODO(gri) Come up with a better representation and API to distinguish
//           between parameterized instantiated and non-instantiated types.

// _TParams returns the type parameters of the named type t, or nil.
// The result is non-nil for an (originally) parameterized type even if it is instantiated.
func (t *Named) _TParams() []*TypeName { return t.tparams }

// _TArgs returns the type arguments after instantiation of the named type t, or nil if not instantiated.
func (t *Named) _TArgs() []Type { return t.targs }

// _SetTArgs sets the type arguments of Named.
func (t *Named) _SetTArgs(args []Type) { t.targs = args }

// NumMethods returns the number of explicit methods whose receiver is named type t.
func (t *Named) NumMethods() int { return len(t.methods) }

// Method returns the i'th method of named type t for 0 <= i < t.NumMethods().
func (t *Named) Method(i int) *Func { return t.methods[i] }

// SetUnderlying sets the underlying type and marks t as complete.
func (t *Named) SetUnderlying(underlying Type) {
	if underlying == nil {
		panic("types.Named.SetUnderlying: underlying type must not be nil")
	}
	if _, ok := underlying.(*Named); ok {
		panic("types.Named.SetUnderlying: underlying type must not be *Named")
	}
	t.underlying = underlying
}

// AddMethod adds method m unless it is already in the method list.
func (t *Named) AddMethod(m *Func) {
	if i, _ := lookupMethod(t.methods, m.pkg, m.name); i < 0 {
		t.methods = append(t.methods, m)
	}
}

// Note: This is a uint32 rather than a uint64 because the
// respective 64 bit atomic instructions are not available
// on all platforms.
var lastId uint32

// nextId returns a value increasing monotonically by 1 with
// each call, starting with 1. It may be called concurrently.
func nextId() uint64 { return uint64(atomic.AddUint32(&lastId, 1)) }

// A _TypeParam represents a type parameter type.
type _TypeParam struct {
	check *Checker  // for lazy type bound completion
	id    uint64    // unique id
	obj   *TypeName // corresponding type name
	index int       // parameter index
	bound Type      // *Named or *Interface; underlying type is always *Interface
}

// newTypeParam returns a new TypeParam.
func (check *Checker) newTypeParam(obj *TypeName, index int, bound Type) *_TypeParam {
	assert(bound != nil)
	typ := &_TypeParam{check: check, id: nextId(), obj: obj, index: index, bound: bound}
	if obj.typ == nil {
		obj.typ = typ
	}
	return typ
}

func (t *_TypeParam) Bound() *Interface {
	iface := asInterface(t.bound)
	if iface == nil {
		return &emptyInterface
	}
	// use the type bound position if we have one
	pos := token.NoPos
	if n, _ := t.bound.(*Named); n != nil {
		pos = n.obj.pos
	}
	// TODO(rFindley) switch this to an unexported method on Checker.
	t.check.completeInterface(pos, iface)
	return iface
}

// optype returns a type's operational type. Except for
// type parameters, the operational type is the same
// as the underlying type (as returned by under). For
// Type parameters, the operational type is determined
// by the corresponding type bound's type list. The
// result may be the bottom or top type, but it is never
// the incoming type parameter.
func optype(typ Type) Type {
	if t := asTypeParam(typ); t != nil {
		// If the optype is typ, return the top type as we have
		// no information. It also prevents infinite recursion
		// via the asTypeParam converter function. This can happen
		// for a type parameter list of the form:
		// (type T interface { type T }).
		// See also issue #39680.
		if u := t.Bound().allTypes; u != nil && u != typ {
			// u != typ and u is a type parameter => under(u) != typ, so this is ok
			return under(u)
		}
		return theTop
	}
	return under(typ)
}

// An instance represents an instantiated generic type syntactically
// (without expanding the instantiation). Type instances appear only
// during type-checking and are replaced by their fully instantiated
// (expanded) types before the end of type-checking.
type instance struct {
	check   *Checker    // for lazy instantiation
	pos     token.Pos   // position of type instantiation; for error reporting only
	base    *Named      // parameterized type to be instantiated
	targs   []Type      // type arguments
	poslist []token.Pos // position of each targ; for error reporting only
	value   Type        // base(targs...) after instantiation or Typ[Invalid]; nil if not yet set
}

// expand returns the instantiated (= expanded) type of t.
// The result is either an instantiated *Named type, or
// Typ[Invalid] if there was an error.
func (t *instance) expand() Type {
	v := t.value
	if v == nil {
		v = t.check.instantiate(t.pos, t.base, t.targs, t.poslist)
		if v == nil {
			v = Typ[Invalid]
>>>>>>> 346b18ee9d15410ab08dd583787c64dbed0666d2
		}
	}

	// types are different
	return nil
}
