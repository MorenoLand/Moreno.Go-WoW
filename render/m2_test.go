package render

import "testing"

func TestM2RenderOrderUsesMaterialPriority(t *testing.T) {
	background := &m2Part{renderOrder: 27, priorityPlane: 10, material: m2RenderFlag{blend: 2}}
	dragon := &m2Part{renderOrder: 4, priorityPlane: 11, material: m2RenderFlag{blend: 2}}
	if m2RenderOrder(background) >= m2RenderOrder(dragon) {
		t.Fatalf("background order=%d dragon order=%d", m2RenderOrder(background), m2RenderOrder(dragon))
	}
	layered := &m2Part{renderOrder: 3, priorityPlane: 11, materialLayer: 1, material: m2RenderFlag{blend: 2}}
	if m2RenderOrder(dragon) >= m2RenderOrder(layered) {
		t.Fatalf("base order=%d layered order=%d", m2RenderOrder(dragon), m2RenderOrder(layered))
	}
	opaque := &m2Part{renderOrder: 4, priorityPlane: 11, material: m2RenderFlag{blend: 0}}
	if m2RenderOrder(opaque) != -96 {
		t.Fatalf("opaque order=%d", m2RenderOrder(opaque))
	}
}

func TestPoseM2VertexUsesSkinBonePalette(t *testing.T) {
	model := parsedM2{bones: []m2Bone{{parent: -1, rotation: [4]float32{0, 0, 0, 1}, scale: [3]float32{1, 1, 1}}, {parent: -1, translation: [3]float32{2, 0, 0}, rotation: [4]float32{0, 0, 0, 1}, scale: [3]float32{1, 1, 1}}}, boneCombos: []uint16{0, 1}}
	skin := parsedSkin{bones: [][4]uint8{{1, 0, 0, 0}}}
	vertex := m2Vertex{weights: [4]uint8{255, 0, 0, 0}, bones: [4]uint8{0, 0, 0, 0}}
	posed := poseM2Vertex(model, skin, 0, vertex, 0)
	if posed.position[0] != 2 {
		t.Fatalf("posed x=%f", posed.position[0])
	}
}
