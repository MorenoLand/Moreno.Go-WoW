package render

import "math"

// segmentBlocked reports whether the ground or a solid (a wall, a floor, a
// large doodad) lies between two points, leaving a little at each end free.
func (collision worldSceneCollision) segmentBlocked(from, to [3]float32) bool {
	dx, dy, dz := to[0]-from[0], to[1]-from[1], to[2]-from[2]
	length := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
	if length < 2 {
		return false
	}
	// The ends are pulled in so that standing on the ground or touching a wall
	// does not count as being behind it.
	inset := float32(0.6) / length
	start := [3]float32{from[0] + dx*inset, from[1] + dy*inset, from[2] + dz*inset}
	end := [3]float32{to[0] - dx*inset, to[1] - dy*inset, to[2] - dz*inset}
	// The ground: sampled along the way.
	steps := int(length / 2.5)
	for step := 1; step < steps; step++ {
		fraction := float32(step) / float32(steps)
		x, y, z := from[0]+dx*fraction, from[1]+dy*fraction, from[2]+dz*fraction
		if height, _, ok := collision.terrain(x, y); ok && z < height-0.35 {
			return true
		}
	}
	// What is built: the triangles of the cells the segment passes through.
	seen := make(map[[2]int]bool, 8)
	cellSteps := int(math.Hypot(float64(dx), float64(dy))/(worldCollisionCell/2)) + 2
	for step := 0; step <= cellSteps; step++ {
		fraction := float32(step) / float32(cellSteps)
		cell := [2]int{int(math.Floor(float64((from[0] + dx*fraction) / worldCollisionCell))), int(math.Floor(float64((from[1] + dy*fraction) / worldCollisionCell)))}
		if seen[cell] {
			continue
		}
		seen[cell] = true
		for _, ref := range collision.cells[cell] {
			triangle := collision.solids[ref.mesh].triangles[ref.triangle]
			if triangle.noCamera {
				continue
			}
			if hit, ok := worldSegmentTriangleHit(start, end, triangle); ok && hit > 0 {
				return true
			}
		}
	}
	return false
}
