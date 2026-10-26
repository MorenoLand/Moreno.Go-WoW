package render

// A bone's whole parent chain (pivot rotate, scale, translate at every
// ancestor) is a single affine map, so the chain can be resolved once per bone
// per frame instead of once per vertex per bone influence. The matrices are
// derived by running the original chain walkers on the origin and the unit
// axes, which keeps their flag handling and cycle guard exactly as they were.

type m2BoneMatrix struct {
	// point maps position: x' = point[0]*x + point[1]*y + point[2]*z + point[3],
	// then the same for rows 1 and 2.
	point [12]float32
	// normal is the rotation-only chain, row-major 3x3.
	normal [9]float32
}

// buildM2BoneMatrices composes each bone's own transform with its parent's
// already resolved matrix. That needs every parent to precede its child, which
// real M2 data has; anything else (including malformed cycles) falls back to
// walking each chain.
func buildM2BoneMatrices(bones []m2Bone, out []m2BoneMatrix) []m2BoneMatrix {
	if cap(out) < len(bones) {
		out = make([]m2BoneMatrix, len(bones))
	}
	out = out[:len(bones)]
	for index := range bones {
		if int(bones[index].parent) >= index {
			return buildM2BoneMatricesByWalk(bones, out)
		}
	}
	axes := [3][3]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for index := range bones {
		bone := &bones[index]
		// The bone's own step: rotate about the pivot, scale per axis, then
		// translate (only when the bone is animated, as in transformM2Point).
		var local m2BoneMatrix
		local.normal = [9]float32{1, 0, 0, 0, 1, 0, 0, 0, 1}
		local.point = [12]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}
		if bone.flags&(0x80|0x200) != 0 {
			for column, axis := range axes {
				rotated := rotateM2Vector(bone.rotation, axis)
				for row := 0; row < 3; row++ {
					local.normal[row*3+column] = rotated[row]
					local.point[row*4+column] = rotated[row] * bone.scale[row]
				}
			}
			for row := 0; row < 3; row++ {
				pivoted := local.point[row*4]*bone.pivot[0] + local.point[row*4+1]*bone.pivot[1] + local.point[row*4+2]*bone.pivot[2]
				local.point[row*4+3] = bone.pivot[row] + bone.translation[row] - pivoted
			}
		}
		if bone.parent < 0 {
			out[index] = local
			continue
		}
		parent := &out[bone.parent]
		var total m2BoneMatrix
		for row := 0; row < 3; row++ {
			for column := 0; column < 3; column++ {
				total.point[row*4+column] = parent.point[row*4]*local.point[column] + parent.point[row*4+1]*local.point[4+column] + parent.point[row*4+2]*local.point[8+column]
				total.normal[row*3+column] = parent.normal[row*3]*local.normal[column] + parent.normal[row*3+1]*local.normal[3+column] + parent.normal[row*3+2]*local.normal[6+column]
			}
			total.point[row*4+3] = parent.point[row*4]*local.point[3] + parent.point[row*4+1]*local.point[7] + parent.point[row*4+2]*local.point[11] + parent.point[row*4+3]
		}
		out[index] = total
	}
	return out
}

// buildM2BoneMatricesByWalk derives each bone's matrix by running the chain
// walkers on the origin and unit axes.
func buildM2BoneMatricesByWalk(bones []m2Bone, out []m2BoneMatrix) []m2BoneMatrix {
	out = out[:len(bones)]
	axes := [3][3]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for index := range bones {
		origin := transformM2Point(index, [3]float32{}, bones, 0)
		matrix := &out[index]
		for column, axis := range axes {
			image := transformM2Point(index, axis, bones, 0)
			normalImage := transformM2Normal(index, axis, bones, 0)
			for row := 0; row < 3; row++ {
				matrix.point[row*4+column] = image[row] - origin[row]
				matrix.normal[row*3+column] = normalImage[row]
			}
		}
		for row := 0; row < 3; row++ {
			matrix.point[row*4+3] = origin[row]
		}
	}
	return out
}

// poseM2VertexWithMatrices is poseM2VertexWithBones using resolved bone
// matrices.
func poseM2VertexWithMatrices(vertex m2Vertex, matrices []m2BoneMatrix) posedM2Vertex {
	var position, normal [3]float32
	weightTotal := 0
	for slot, weight := range vertex.weights {
		if weight == 0 {
			continue
		}
		boneIndex := int(vertex.bones[slot])
		if boneIndex >= len(matrices) {
			continue
		}
		weightTotal += int(weight)
		m := &matrices[boneIndex]
		p, n := vertex.position, vertex.normal
		factor := float32(weight) / 255
		for row := 0; row < 3; row++ {
			position[row] += (m.point[row*4]*p[0] + m.point[row*4+1]*p[1] + m.point[row*4+2]*p[2] + m.point[row*4+3]) * factor
			normal[row] += (m.normal[row*3]*n[0] + m.normal[row*3+1]*n[1] + m.normal[row*3+2]*n[2]) * factor
		}
	}
	if weightTotal == 0 {
		return posedM2Vertex{position: vertex.position, normal: vertex.normal}
	}
	if weightTotal != 255 {
		factor := 255 / float32(weightTotal)
		for axis := 0; axis < 3; axis++ {
			position[axis] *= factor
			normal[axis] *= factor
		}
	}
	return posedM2Vertex{position: position, normal: normal}
}
