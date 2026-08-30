// Package ui implements the interface file runtime used by the original
// client's glue screens: table-of-contents loading, XML widget definition
// parsing, Lua 5.1 script execution, widget objects, and the glue API.
//
// Loader semantics follow the original 3.3.5 client paths documented in the
// project research notes (GlueXML.toc entry loop, Include/Script dispatch,
// virtual templates, $parent name substitution, "@%s" chunk names).
package ui

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// xmlNode is a generic XML element: name, attributes, children, and text
// content. Inline script bodies are the text content of script elements.
type xmlNode struct {
	name     string
	attrs    map[string]string
	children []*xmlNode
	text     strings.Builder
}

func (n *xmlNode) attr(name string) (string, bool) {
	v, ok := n.attrs[name]
	return v, ok
}

func (n *xmlNode) attrDefault(name, def string) string {
	if v, ok := n.attrs[name]; ok {
		return v
	}
	return def
}

func (n *xmlNode) child(name string) *xmlNode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *xmlNode) childText(name string) string {
	if c := n.child(name); c != nil {
		return c.text.String()
	}
	return ""
}

// parseXML decodes XML source into a node tree. A leading UTF-8 BOM is
// skipped the way the original TOC/XML readers skip it. Element names use
// the local part only, so the interface XML namespace does not affect
// lookup. Comments and processing instructions are dropped; character data
// is kept in document order so inline script bodies survive intact.
func parseXML(source []byte) (*xmlNode, error) {
	if len(source) >= 3 && source[0] == 0xEF && source[1] == 0xBB && source[2] == 0xBF {
		source = source[3:]
	}
	decoder := xml.NewDecoder(strings.NewReader(string(source)))
	decoder.Strict = false
	var root *xmlNode
	stack := make([]*xmlNode, 0, 16)
	for {
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		switch tok := token.(type) {
		case xml.StartElement:
			node := &xmlNode{name: tok.Name.Local, attrs: make(map[string]string, len(tok.Attr))}
			for _, a := range tok.Attr {
				node.attrs[a.Name.Local] = a.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("multiple root elements")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(tok)
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("no root element")
	}
	return root, nil
}

// scriptParams returns the implicit parameter list the original client
// compiles XML script bodies with for each script handler type.
func scriptParams(handler string) string {
	switch handler {
	case "OnLoad":
		return "self"
	case "OnEvent":
		return "self, event, ..."
	case "OnUpdate":
		return "self, elapsed"
	case "OnClick":
		return "self, button, down"
	case "OnDoubleClick":
		return "self, button"
	case "OnEnter", "OnLeave":
		return "self, motion"
	case "OnKeyDown", "OnKeyUp":
		return "self, key"
	case "OnChar":
		return "self, text"
	case "OnEscapePressed", "OnEnterPressed", "OnSpacePressed", "OnArrowPressed",
		"OnTabPressed", "OnEditFocusGained", "OnEditFocusLost", "OnUpdateModel":
		return "self"
	case "OnTextChanged":
		return "self, changed"
	case "OnValueChanged":
		return "self, value"
	case "OnVerticalScroll":
		return "self, offset"
	case "OnScrollRangeChanged":
		return "self, x, y"
	case "OnMouseWheel":
		return "self, delta"
	case "OnMouseDown", "OnMouseUp":
		return "self, button"
	case "OnHyperlinkClick":
		return "self, link, text, button"
	case "OnMovieShowSubtitle":
		return "self, text"
	case "OnMovieHideSubtitle", "OnMovieFinished":
		return "self"
	default:
		return "self, ..."
	}
}

// resolveParentName expands the "$parent" prefix in a child element name
// against the enclosing widget name, e.g. "$parentBGHighlight" under
// AccountNameButton becomes AccountNameButtonBGHighlight.
func resolveParentName(name, parent string) string {
	if parent != "" && strings.HasPrefix(name, "$parent") {
		return parent + strings.TrimPrefix(name, "$parent")
	}
	return name
}
