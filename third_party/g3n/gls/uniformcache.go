//go:build !wasm
// +build !wasm

package gls

import (
	"math"
	"unsafe"
)

// A program keeps the value of each of its uniforms, so setting one to the value
// it already holds only costs a call into the driver. These caches remember the
// last value set per program and location and skip such calls.

const uniformCacheFloats = 32

type uniformKey struct {
	program  uint32
	location int32
}

type uniformValue struct {
	kind   uint8 // which setter, so a vec3 is not mistaken for a float array
	length uint8
	bits   [uniformCacheFloats]uint32
}

// uniformHolds reports whether the uniform already holds the n values, and
// remembers them if not.
func (gs *GLS) uniformHolds(location int32, kind uint8, n int, bits *[uniformCacheFloats]uint32) bool {

	if gs.prog == nil || location < 0 {
		return false
	}
	if gs.uniforms == nil {
		gs.uniforms = make(map[uniformKey]uniformValue)
	}
	key := uniformKey{gs.prog.handle, location}
	held, ok := gs.uniforms[key]
	if ok && held.kind == kind && int(held.length) == n && held.bits == *bits {
		return true
	}
	gs.uniforms[key] = uniformValue{kind: kind, length: uint8(n), bits: *bits}
	return false
}

func (gs *GLS) floatsHeld(location int32, kind uint8, v0, v1, v2, v3 float32, n int) bool {

	var bits [uniformCacheFloats]uint32
	bits[0], bits[1], bits[2], bits[3] = math.Float32bits(v0), math.Float32bits(v1), math.Float32bits(v2), math.Float32bits(v3)
	return gs.uniformHolds(location, kind, n, &bits)
}

// floatArrayHeld is floatsHeld for count*width floats at v; arrays too long to
// remember are never reported as held.
func (gs *GLS) floatArrayHeld(location int32, kind uint8, count int32, width int, v *float32) bool {

	n := int(count) * width
	if n <= 0 || n > uniformCacheFloats {
		return false
	}
	values := unsafe.Slice(v, n)
	var bits [uniformCacheFloats]uint32
	for index, value := range values {
		bits[index] = math.Float32bits(value)
	}
	return gs.uniformHolds(location, kind, n, &bits)
}

// forgetUniforms drops what is remembered of a program (it was linked again or
// deleted, so its uniforms hold nothing set before).
func (gs *GLS) forgetUniforms(program uint32) {

	for key := range gs.uniforms {
		if key.program == program {
			delete(gs.uniforms, key)
		}
	}
}

const (
	uniformKindInt uint8 = iota + 1
	uniformKindFloat
	uniformKindVec2
	uniformKindVec3
	uniformKindVec4
	uniformKindFloatArray
	uniformKindVec2Array
	uniformKindVec3Array
	uniformKindVec4Array
)
