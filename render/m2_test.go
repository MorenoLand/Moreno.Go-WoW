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
