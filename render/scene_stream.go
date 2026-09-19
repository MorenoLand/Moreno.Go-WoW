package render

import "github.com/g3n/engine/core"

func queueSceneLeaves(node core.INode, queue *[]core.INode) {
	if node == nil {
		return
	}
	for _, child := range node.Children() {
		if len(child.Children()) == 0 {
			child.SetVisible(false)
			*queue = append(*queue, child)
			continue
		}
		queueSceneLeaves(child, queue)
	}
}

func revealSceneBatch(queue *[]core.INode, batch int) {
	if queue == nil || batch < 1 {
		return
	}
	if batch > len(*queue) {
		batch = len(*queue)
	}
	for _, node := range (*queue)[:batch] {
		node.SetVisible(true)
	}
	*queue = (*queue)[batch:]
}
