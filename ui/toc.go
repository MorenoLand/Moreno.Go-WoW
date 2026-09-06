package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// Loader reads interface files from a directory tree laid out like the
// client's Interface data (Interface\GlueXML\...). File lookups are
// case-insensitive with forward or back slashes, matching the client's
// MPQ-backed path handling.
type Loader struct {
	Root      string
	assetRoot string
	mpq       *mpqSet
	rt        *Runtime
}

// NewLoader creates a loader rooted at the given data directory.
func NewLoader(root string, rt *Runtime) *Loader {
	l := &Loader{Root: root, assetRoot: root, rt: rt}
	installTemplateFactory(l)
	return l
}

func NewLoaderWithAssets(root, assetRoot string, rt *Runtime) *Loader {
	l := &Loader{Root: root, assetRoot: assetRoot, rt: rt}
	installTemplateFactory(l)
	return l
}

func NewMPQLoader(dataPath, locale string, rt *Runtime) (*Loader, error) {
	archives, err := openMPQSet(dataPath, locale)
	if err != nil {
		return nil, err
	}
	l := &Loader{mpq: archives, rt: rt}
	rt.Glue.AddOns = discoverAddOns(dataPath)
	installTemplateFactory(l)
	return l, nil
}

func installTemplateFactory(l *Loader) {
	l.rt.instantiateTemplate = func(w *widget, template string) {
		tpl, ok := l.rt.virtuals[template]
		if !ok {
			return
		}
		merged := mergeTemplate(l, tpl, &xmlNode{name: w.objectType(), attrs: map[string]string{}})
		l.applyWidgetAttrs(w, merged)
		if w.kind == kindTexture {
			w.textureFile = merged.attrDefault("file", w.textureFile)
			w.alphaMode = merged.attrDefault("alphaMode", w.alphaMode)
			w.horizTile = parseBool(merged.attrDefault("horizTile", "false"), w.horizTile)
			w.vertTile = parseBool(merged.attrDefault("vertTile", "false"), w.vertTile)
		} else if w.kind == kindFontString {
			w.fontObject = merged.attrDefault("inherits", w.fontObject)
			w.justifyH = merged.attrDefault("justifyH", w.justifyH)
			w.justifyV = merged.attrDefault("justifyV", w.justifyV)
		}
		for _, group := range merged.children {
			switch group.name {
			case "Size":
				w.width = attrFloat(group, "x", w.width)
				w.height = attrFloat(group, "y", w.height)
				if d := group.child("AbsDimension"); d != nil {
					w.width = attrFloat(d, "x", w.width)
					w.height = attrFloat(d, "y", w.height)
				}
			case "Anchors":
				w.points = append(w.points, parseAnchors(group, w.parentName())...)
			case "Backdrop":
				w.backdrop = parseBackdrop(group)
			case "HitRectInsets":
				if inset := group.child("AbsInset"); inset != nil {
					w.hitInsetL = attrFloat(inset, "left", 0)
					w.hitInsetR = attrFloat(inset, "right", 0)
					w.hitInsetT = attrFloat(inset, "top", 0)
					w.hitInsetB = attrFloat(inset, "bottom", 0)
				}
			case "Layers":
				for _, layerEl := range group.children {
					for _, region := range layerEl.children {
						if strings.EqualFold(region.name, "Texture") || strings.EqualFold(region.name, "FontString") {
							if child, err := l.buildRegion(region, w, "CreateFrame:"+template); err == nil {
								child.layerLevel = layerOrder(layerEl.attrDefault("level", "ARTWORK"))
							}
						}
					}
				}
			case "Frames":
				for _, frameEl := range group.children {
					if isWidgetElement(frameEl.name) {
						if child, err := l.buildWidget(frameEl, w, "CreateFrame:"+template); err == nil {
							addWidgetChild(w, child)
							l.bindParentKey(w, child, frameEl)
						}
					}
				}
			case "NormalTexture", "PushedTexture", "HighlightTexture", "DisabledTexture",
				"CheckedTexture", "DisabledCheckedTexture", "ThumbTexture":
				texture := l.buildButtonTexture(group, w, "CreateFrame:"+template)
				switch group.name {
				case "NormalTexture":
					w.normalTexture = texture
				case "PushedTexture":
					w.pushedTexture = texture
				case "HighlightTexture":
					w.highlightTexture = texture
				case "DisabledTexture":
					w.disabledTexture = texture
				case "CheckedTexture":
					w.checkedTexture = texture
				case "DisabledCheckedTexture":
					w.disabledCheckedTexture = texture
				}
				if texture.name != "" {
					l.rt.register(texture)
				}
				l.bindParentKey(w, texture, group)
			case "NormalFont":
				w.normalFont = group.attrDefault("style", "")
			case "HighlightFont":
				w.highlightFont = group.attrDefault("style", "")
			case "DisabledFont":
				w.disabledFont = group.attrDefault("style", "")
			case "ButtonText":
				label := buttonTextNode(group, w, merged)
				if region, err := l.buildRegion(label, w, "CreateFrame:"+template); err == nil {
					w.buttonLabel = region
				}
			case "Scripts":
				for _, scriptEl := range group.children {
					handler := scriptEl.name
					if fnName, ok := scriptEl.attr("function"); ok {
						w.scripts[handler] = l.namedHandler(fnName)
						continue
					}
					body := strings.TrimSpace(scriptEl.text.String())
					if body == "" {
						continue
					}
					source := "@" + "CreateFrame:" + template + ":" + handler
					if fn := l.compileScript(body, handler, source); fn != nil {
						w.scripts[handler] = fn
					}
				}
			}
		}
	}
}

// resolve maps an interface path (e.g. Interface\GlueXML\GlueXML.toc) to a
// file under the loader root, tolerating case and separator differences.
func (l *Loader) resolve(interfacePath string) (string, error) {
	return l.resolveAt(l.Root, interfacePath)
}

func (l *Loader) resolveAt(root, interfacePath string) (string, error) {
	clean := filepath.FromSlash(strings.ReplaceAll(interfacePath, "/", "\\"))
	candidate := filepath.Join(root, clean)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	parts := strings.Split(strings.Trim(clean, "\\"), "\\")
	dir := root
	for i, part := range parts {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		found := ""
		for _, e := range entries {
			if strings.EqualFold(e.Name(), part) {
				found = filepath.Join(dir, e.Name())
				break
			}
		}
		if found == "" {
			return "", fs.ErrNotExist
		}
		if i == len(parts)-1 {
			info, err := os.Stat(found)
			if err == nil && info.IsDir() {
				return "", fs.ErrNotExist
			}
			return found, nil
		}
		dir = found
	}
	return "", fs.ErrNotExist
}

// ReadAsset resolves and reads an interface asset path, tolerating a
// missing .blp extension the way the client's texture loader does.
func (l *Loader) ReadAsset(path string) ([]byte, error) {
	data, err := l.readAsset(path)
	if err == nil {
		return data, nil
	}
	return l.readAsset(path + ".blp")
}

func (l *Loader) ReadFile(path string) ([]byte, error) { return l.read(path) }

func (l *Loader) Close() error {
	if l.mpq == nil {
		return nil
	}
	return l.mpq.Close()
}

// read resolves and reads an interface file.
func (l *Loader) read(interfacePath string) ([]byte, error) {
	if l.mpq != nil {
		return l.mpq.ReadFile(interfacePath)
	}
	path, err := l.resolve(interfacePath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (l *Loader) readAsset(interfacePath string) ([]byte, error) {
	if l.mpq != nil {
		return l.mpq.ReadFile(interfacePath)
	}
	path, err := l.resolveAt(l.assetRoot, interfacePath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// normalizePath collapses standalone ".." segments between separators, the
// way the client's interface file loader does before opening a file.
func normalizePath(p string) string {
	p = strings.ReplaceAll(p, "/", "\\")
	for {
		idx := strings.Index(p, "\\..\\")
		dotdot := -1
		if idx >= 0 {
			dotdot = idx + 1
		} else if strings.HasSuffix(p, "\\..") {
			dotdot = len(p) - 2
		}
		if dotdot < 0 {
			break
		}
		prev := strings.LastIndex(p[:dotdot-1], "\\")
		if prev < 0 {
			break
		}
		end := dotdot + 2
		if end < len(p) {
			p = p[:prev+1] + p[end+1:]
		} else {
			p = p[:prev]
		}
	}
	return p
}

// LoadTOC executes a table of contents: every non-comment line names a file
// relative to the TOC's own directory, entries load in order, and the
// optional progress callback reports (done, total) after each entry. Entry
// counting matches the client: non-empty lines that do not start with '#'.
func (l *Loader) LoadTOC(interfaceTOC string, progress func(done, total int)) error {
	data, err := l.read(interfaceTOC)
	if err != nil {
		return fmt.Errorf("open %s: %w", interfaceTOC, err)
	}
	content := strings.TrimPrefix(string(data), "\xEF\xBB\xBF")
	lines := strings.Split(content, "\n")
	entries := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if line == "" || strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		entries = append(entries, strings.TrimRight(line, " \t"))
	}
	for i, entry := range entries {
		path := entry
		if !isInterfacePath(entry) {
			path = dirOf(interfaceTOC) + entry
		}
		path = normalizePath(path)
		if err := l.LoadInterfaceFile(path); err != nil {
			return fmt.Errorf("%s: entry %q: %w", interfaceTOC, entry, err)
		}
		if progress != nil {
			progress(i+1, len(entries))
		}
	}
	return nil
}

func isInterfacePath(p string) bool {
	return strings.HasPrefix(strings.ToLower(p), "interface\\") || strings.HasPrefix(strings.ToLower(p), "interface/")
}

// LoadInterfaceFile dispatches one interface file by extension: .lua files
// execute as script chunks, anything else parses as interface XML. Include
// elements recurse through this function with paths resolved against the
// including file's directory.
func (l *Loader) LoadInterfaceFile(interfacePath string) error {
	interfacePath = normalizePath(interfacePath)
	data, err := l.read(interfacePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", interfacePath, err)
	}
	if strings.HasSuffix(strings.ToLower(interfacePath), ".lua") {
		l.rt.doFileBody(string(data), interfacePath)
		return nil
	}
	root, err := parseXML(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", interfacePath, err)
	}
	if root.name != "Ui" {
		return fmt.Errorf("%s: root element is %s, want Ui", interfacePath, root.name)
	}
	return l.loadUiChildren(root, interfacePath)
}

// loadUiChildren walks the children of a Ui element: Script and Include
// load files, Font defines a named font object, and widget elements either
// register a virtual template or create a live widget.
func (l *Loader) loadUiChildren(ui *xmlNode, interfacePath string) error {
	for _, child := range ui.children {
		switch child.name {
		case "Script":
			if file, ok := child.attr("file"); ok {
				resolved := resolveRelative(interfacePath, file)
				data, err := l.read(resolved)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						continue
					}
					return fmt.Errorf("open %s: %w", resolved, err)
				}
				l.rt.doFileBody(string(data), resolved)
			} else if body := strings.TrimSpace(child.text.String()); body != "" {
				// Inline script body directly under Ui.
				l.rt.Execute(body, "@"+interfacePath+":<Script>")
			} else {
				return fmt.Errorf("%s: Element 'Script' without file attribute", interfacePath)
			}
		case "Include":
			file, ok := child.attr("file")
			if !ok {
				return fmt.Errorf("%s: Element 'Include' without file attribute", interfacePath)
			}
			resolved := resolveRelative(interfacePath, file)
			if err := l.LoadInterfaceFile(resolved); err != nil {
				return err
			}
		case "Font":
			name := child.attrDefault("name", "")
			if name == "" {
				return fmt.Errorf("%s: Unnamed font node at top level", interfacePath)
			}
			font := &Font{Name: name}
			if inherits, ok := child.attr("inherits"); ok && inherits != "" {
				if parent, ok := l.rt.fonts[inherits]; ok {
					*font = *parent
				}
			}
			font.Name = name
			if v, ok := child.attr("font"); ok {
				font.FontFile = v
			}
			font.JustifyH = child.attrDefault("justifyH", font.JustifyH)
			font.JustifyV = child.attrDefault("justifyV", font.JustifyV)
			if v, ok := child.attr("outline"); ok {
				font.Outline = v
			}
			if shadow := child.child("Shadow"); shadow != nil {
				font.Shadow = true
				if offset := shadow.child("Offset"); offset != nil {
					dimension := offset.child("AbsDimension")
					if dimension != nil {
						font.ShadowOffsetX = attrFloat(dimension, "x", font.ShadowOffsetX)
						font.ShadowOffsetY = attrFloat(dimension, "y", font.ShadowOffsetY)
					}
				}
				if shadowColor := shadow.child("Color"); shadowColor != nil {
					font.ShadowColor = rgba{attrFloat(shadowColor, "r", 0), attrFloat(shadowColor, "g", 0), attrFloat(shadowColor, "b", 0), attrFloat(shadowColor, "a", 1)}
				}
			}
			if h := child.child("FontHeight"); h != nil {
				font.Height = attrFloat(h.child("AbsValue"), "val", font.Height)
			}
			if c := child.child("Color"); c != nil {
				font.Color = rgba{attrFloat(c, "r", 0), attrFloat(c, "g", 0), attrFloat(c, "b", 0), 1}
			}
			l.rt.fonts[name] = font
			fontTable := l.rt.L.NewTable()
			fontTable.RawSetString("name", lua.LString(name))
			fontTable.RawSetString("height", lua.LNumber(font.Height))
			fontTable.RawSetString("font", lua.LString(font.FontFile))
			fontTable.RawSetString("GetTextColor", l.rt.L.NewFunction(func(L *lua.LState) int {
				L.Push(lua.LNumber(font.Color.r))
				L.Push(lua.LNumber(font.Color.g))
				L.Push(lua.LNumber(font.Color.b))
				L.Push(lua.LNumber(font.Color.a))
				return 4
			}))
			l.rt.L.SetGlobal(name, fontTable)
		default:
			if isWidgetElement(child.name) {
				if err := l.instantiateTopLevel(child, interfacePath); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// instantiateTopLevel handles a widget element directly under Ui: virtual
// widgets register as templates by name, live widgets are created.
func (l *Loader) instantiateTopLevel(node *xmlNode, interfacePath string) error {
	virtual := parseBool(node.attrDefault("virtual", "false"), false)
	name := node.attrDefault("name", "")
	if virtual {
		if name == "" {
			return fmt.Errorf("%s: Unnamed virtual node at top level", interfacePath)
		}
		l.rt.virtuals[name] = node
		return nil
	}
	_, err := l.buildWidget(node, nil, interfacePath)
	return err
}

func isWidgetElement(name string) bool {
	switch name {
	case "Frame", "Button", "CheckButton", "EditBox", "Slider", "ScrollFrame", "ScrollingMessageFrame",
		"SimpleHTML", "Model", "ModelFFX", "MovieFrame", "Texture", "FontString":
		return true
	}
	return false
}

// buildWidget constructs a widget from its XML node, applying template
// inheritance first, then the node's own definition. Regions, child frames,
// and scripts are built depth-first; OnLoad fires after the whole subtree
// exists, which is the order scripts rely on when they look up named
// subregions from OnLoad.
func (l *Loader) buildWidget(node *xmlNode, parent *widget, interfacePath string) (*widget, error) {
	merged := node
	if inherits, ok := node.attr("inherits"); ok && inherits != "" {
		parts := inheritanceNames(inherits)
		if len(parts) == 0 {
			return nil, fmt.Errorf("%s: template %q not found", interfacePath, inherits)
		}
		base, found := l.rt.virtuals[parts[0]]
		if !found {
			return nil, fmt.Errorf("%s: template %q not found", interfacePath, parts[0])
		}
		for _, part := range parts[1:] {
			tpl, ok := l.rt.virtuals[part]
			if !ok {
				return nil, fmt.Errorf("%s: template %q not found", interfacePath, part)
			}
			base = mergeTemplate(l, tpl, base)
		}
		merged = mergeTemplate(l, base, node)
	}

	kind := kindFromObjectType(node.name)
	name := node.attrDefault("name", "")
	if parent != nil {
		name = resolveParentName(name, parent.name)
	}
	w := newWidget(kind, name)
	w.parent = parent
	l.applyWidgetAttrs(w, merged)
	if parent != nil {
		if _, ok := merged.attr("frameStrata"); !ok {
			w.frameStrata = parent.frameStrata
		}
	}
	if parent != nil && parent.kind == kindScrollFrame {
		parent.scrollChild = w
	}
	if w.parent == nil && w.topLevel {
		if root := l.rt.lookup("GlueParent"); root != nil {
			w.parent = root
			addWidgetChild(root, w)
		}
	}

	for _, group := range merged.children {
		switch group.name {
		case "Size":
			w.width = attrFloat(group, "x", w.width)
			w.height = attrFloat(group, "y", w.height)
			if d := group.child("AbsDimension"); d != nil {
				w.width = attrFloat(d, "x", w.width)
				w.height = attrFloat(d, "y", w.height)
			}
		case "Anchors":
			w.points = append(w.points, parseAnchors(group, w.parentName())...)
		case "Backdrop":
			w.backdrop = parseBackdrop(group)
		case "Layers":
			for _, layerEl := range group.children {
				for _, region := range layerEl.children {
					if strings.EqualFold(region.name, "Texture") || strings.EqualFold(region.name, "FontString") {
						child, err := l.buildRegion(region, w, interfacePath)
						if err != nil {
							return nil, err
						}
						child.layerLevel = layerOrder(layerEl.attrDefault("level", "ARTWORK"))
					}
				}
			}
		case "Texture", "FontString", "Fontstring":
			child, err := l.buildRegion(group, w, interfacePath)
			if err != nil {
				return nil, err
			}
			child.layerLevel = layerArtwork
		case "Frames":
			for _, frameEl := range group.children {
				if isWidgetElement(frameEl.name) {
					child, err := l.buildWidget(frameEl, w, interfacePath)
					if err != nil {
						return nil, err
					}
					addWidgetChild(w, child)
					l.bindParentKey(w, child, frameEl)
				}
			}
		case "ScrollChild":
			for _, frameEl := range group.children {
				if isWidgetElement(frameEl.name) {
					child, err := l.buildWidget(frameEl, w, interfacePath)
					if err != nil {
						return nil, err
					}
					addWidgetChild(w, child)
					l.bindParentKey(w, child, frameEl)
				}
			}
		case "Scripts":
			for _, scriptEl := range group.children {
				handler := scriptEl.name
				if fnName, ok := scriptEl.attr("function"); ok {
					// Handler bound to a named global function.
					w.scripts[handler] = l.namedHandler(fnName)
					continue
				}
				body := strings.TrimSpace(scriptEl.text.String())
				if body == "" {
					continue
				}
				source := "@" + interfacePath + ":" + handler
				if fn := l.compileScript(body, handler, source); fn != nil {
					w.scripts[handler] = fn
				}
			}
		case "NormalTexture", "PushedTexture", "HighlightTexture", "DisabledTexture",
			"CheckedTexture", "DisabledCheckedTexture", "ThumbTexture":
			texture := l.buildButtonTexture(group, w, interfacePath)
			switch group.name {
			case "NormalTexture":
				w.normalTexture = texture
			case "PushedTexture":
				w.pushedTexture = texture
			case "HighlightTexture":
				w.highlightTexture = texture
			case "DisabledTexture":
				w.disabledTexture = texture
			case "CheckedTexture":
				w.checkedTexture = texture
			case "DisabledCheckedTexture":
				w.disabledCheckedTexture = texture
			case "ThumbTexture":
				w.thumbTexture = texture
			}
			if texture.name != "" {
				l.rt.register(texture)
			}
			l.bindParentKey(w, texture, group)
		case "NormalFont":
			w.normalFont = group.attrDefault("style", "")
		case "HighlightFont":
			w.highlightFont = group.attrDefault("style", "")
		case "DisabledFont":
			w.disabledFont = group.attrDefault("style", "")
		case "ButtonText":
			if text := group.attrDefault("text", ""); text != "" {
				w.text = text
			}
			label := buttonTextNode(group, w, merged)
			region, err := l.buildRegion(label, w, interfacePath)
			if err != nil {
				return nil, err
			}
			w.buttonLabel = region
		case "TextInsets":
			if inset := group.child("AbsInset"); inset != nil {
				w.textInsetL = attrFloat(inset, "left", 0)
				w.textInsetR = attrFloat(inset, "right", 0)
				w.textInsetT = attrFloat(inset, "top", 0)
				w.textInsetB = attrFloat(inset, "bottom", 0)
				w.textInsetsSet = true
			}
		case "HitRectInsets":
			if inset := group.child("AbsInset"); inset != nil {
				w.hitInsetL = attrFloat(inset, "left", 0)
				w.hitInsetR = attrFloat(inset, "right", 0)
				w.hitInsetT = attrFloat(inset, "top", 0)
				w.hitInsetB = attrFloat(inset, "bottom", 0)
			}
		case "Attributes":
			// XML Attributes seed frame fields (UIParent panel offsets). Apply
			// without firing OnAttributeChanged so load-time setup does not
			// recurse through UpdateUIPanelPositions before layout exists.
			fields := w.ensureFields(l.rt.L)
			for _, attrEl := range group.children {
				if !strings.EqualFold(attrEl.name, "Attribute") {
					continue
				}
				name := attrEl.attrDefault("name", "")
				if name == "" {
					continue
				}
				raw := attrEl.attrDefault("value", "")
				typ := strings.ToLower(attrEl.attrDefault("type", "string"))
				var value lua.LValue = lua.LString(raw)
				switch typ {
				case "number":
					if f, err := strconv.ParseFloat(raw, 64); err == nil {
						value = lua.LNumber(f)
					}
				case "boolean", "bool":
					value = lua.LBool(parseBool(raw, false))
				case "nil":
					value = lua.LNil
				}
				fields.RawSetString(name, value)
			}
		default:
			// Remaining child elements (insets, scroll children, model
			// tuning) carry visual state the headless runtime does not
			// consume yet.
		}
	}
	if w.buttonLabel != nil {
		if w.buttonLabel.width == 0 && !w.buttonLabel.autoTextWidth {
			w.buttonLabel.width = w.width
		}
		if w.buttonLabel.height == 0 && !w.buttonLabel.autoTextHeight {
			w.buttonLabel.height = w.height
		}
	}
	l.rt.register(w)
	l.bindParentKey(parent, w, merged)
	l.rt.fireHandler(w, "OnLoad")
	return w, nil
}

func (l *Loader) bindParentKey(parent, child *widget, node *xmlNode) {
	if parent == nil || child == nil || node == nil {
		return
	}
	if key := node.attrDefault("parentKey", ""); key != "" {
		parent.ensureFields(l.rt.L).RawSetString(key, child.luaValue(l.rt.L))
	}
}

// buildButtonTexture creates an unnamed texture region from a button
// texture element.
func (l *Loader) buildButtonTexture(node *xmlNode, parent *widget, interfacePath string) *widget {
	merged := node
	if inherits, ok := node.attr("inherits"); ok && inherits != "" {
		if tpl, ok := l.rt.virtuals[inherits]; ok {
			merged = mergeTemplate(l, tpl, node)
		}
	}
	w := newWidget(kindTexture, resolveParentName(merged.attrDefault("name", ""), parent.name))
	w.parent = parent
	w.textureFile = merged.attrDefault("file", "")
	w.alphaMode = merged.attrDefault("alphaMode", "")
	w.horizTile = parseBool(merged.attrDefault("horizTile", "false"), false)
	w.vertTile = parseBool(merged.attrDefault("vertTile", "false"), false)
	if hidden, ok := merged.attr("hidden"); ok {
		w.shown = !parseBool(hidden, false)
	}
	if tc := merged.child("TexCoords"); tc != nil {
		w.texCoordL = attrFloat(tc, "left", 0)
		w.texCoordR = attrFloat(tc, "right", 1)
		w.texCoordT = attrFloat(tc, "top", 0)
		w.texCoordB = attrFloat(tc, "bottom", 1)
	}
	if a := merged.child("Anchors"); a != nil {
		w.points = parseAnchors(a, parent.name)
	}
	if s := merged.child("Size"); s != nil {
		w.width = attrFloat(s, "x", w.width)
		w.height = attrFloat(s, "y", w.height)
		if d := s.child("AbsDimension"); d != nil {
			w.width = attrFloat(d, "x", w.width)
			w.height = attrFloat(d, "y", w.height)
		}
	}
	if v, ok := merged.attr("setAllPoints"); (ok && parseBool(v, false)) || (w.width == 0 && w.height == 0 && len(w.points) == 0) {
		w.points = []anchorPoint{
			{point: "TOPLEFT", relativePoint: "TOPLEFT"},
			{point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"},
		}
	}
	return w
}

// buildRegion creates a Texture or FontString region under a frame.
func (l *Loader) buildRegion(node *xmlNode, parent *widget, interfacePath string) (*widget, error) {
	merged := node
	if inherits, ok := merged.attr("inherits"); ok && inherits != "" {
		if tpl, ok := l.rt.virtuals[inherits]; ok {
			merged = mergeTemplate(l, tpl, node)
		}
	}
	kind := kindTexture
	if strings.EqualFold(merged.name, "FontString") {
		kind = kindFontString
	}
	w := newWidget(kind, resolveParentName(merged.attrDefault("name", ""), parent.name))
	w.parent = parent
	if parent != nil {
		w.frameStrata = parent.frameStrata
	}
	if kind == kindTexture {
		w.textureFile = merged.attrDefault("file", "")
		w.alphaMode = merged.attrDefault("alphaMode", "")
		w.horizTile = parseBool(merged.attrDefault("horizTile", "false"), false)
		w.vertTile = parseBool(merged.attrDefault("vertTile", "false"), false)
		if tc := merged.child("TexCoords"); tc != nil {
			w.texCoordL = attrFloat(tc, "left", 0)
			w.texCoordR = attrFloat(tc, "right", 1)
			w.texCoordT = attrFloat(tc, "top", 0)
			w.texCoordB = attrFloat(tc, "bottom", 1)
		}
		if c := merged.child("Color"); c != nil {
			w.vertexColor = rgba{attrFloat(c, "r", 0), attrFloat(c, "g", 0), attrFloat(c, "b", 0), attrFloat(c, "a", 1)}
		}
	} else {
		w.text = merged.attrDefault("text", "")
		w.fontObject = merged.attrDefault("inherits", "")
		w.justifyH = merged.attrDefault("justifyH", "")
		w.justifyV = merged.attrDefault("justifyV", "")
		w.nonSpaceWrap = parseBool(merged.attrDefault("nonspacewrap", "false"), false)
		if maxLines, ok := merged.attr("maxLines"); ok {
			w.maxLines, _ = strconv.Atoi(maxLines)
		}
		w.autoTextWidth = true
		w.autoTextHeight = true
		if c := merged.child("Color"); c != nil {
			w.textColor = rgba{attrFloat(c, "r", 0), attrFloat(c, "g", 0), attrFloat(c, "b", 0), 1}
		}
	}
	if s := merged.child("Size"); s != nil {
		w.width = attrFloat(s, "x", w.width)
		w.height = attrFloat(s, "y", w.height)
		if _, ok := s.attr("x"); ok && attrFloat(s, "x", 0) > 0 {
			w.explicitWidth = true
		}
		if _, ok := s.attr("y"); ok && attrFloat(s, "y", 0) > 0 {
			w.explicitHeight = true
		}
		if d := s.child("AbsDimension"); d != nil {
			w.width = attrFloat(d, "x", w.width)
			w.height = attrFloat(d, "y", w.height)
			if _, ok := d.attr("x"); ok && attrFloat(d, "x", 0) > 0 {
				w.explicitWidth = true
			}
			if _, ok := d.attr("y"); ok && attrFloat(d, "y", 0) > 0 {
				w.explicitHeight = true
			}
		}
	}
	if kind == kindFontString {
		if w.explicitWidth {
			w.autoTextWidth = false
		}
		if w.explicitHeight {
			w.autoTextHeight = false
		}
	}
	if a := merged.child("Anchors"); a != nil {
		w.points = parseAnchors(a, parent.name)
	}
	if v, ok := merged.attr("setAllPoints"); (ok && parseBool(v, false)) || (w.width == 0 && w.height == 0 && len(w.points) == 0 && kind == kindTexture) {
		w.points = []anchorPoint{
			{point: "TOPLEFT", relativePoint: "TOPLEFT"},
			{point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"},
		}
	}
	if hidden, ok := merged.attr("hidden"); ok {
		w.shown = !parseBool(hidden, false)
	}
	addWidgetChild(parent, w)
	l.bindParentKey(parent, w, merged)
	l.rt.register(w)
	return w, nil
}

func layerOrder(level string) int {
	switch strings.ToUpper(level) {
	case "BACKGROUND":
		return layerBackground
	case "BORDER":
		return layerBorder
	case "OVERLAY":
		return layerOverlay
	case "HIGHLIGHT":
		return layerHighlight
	default:
		return layerArtwork
	}
}

func buttonTextNode(group *xmlNode, parent *widget, button *xmlNode) *xmlNode {
	attrs := make(map[string]string, len(group.attrs)+2)
	for key, value := range group.attrs {
		attrs[key] = value
	}
	if attrs["text"] == "" && parent.text != "" {
		attrs["text"] = parent.text
	}
	if attrs["inherits"] == "" {
		if normal := button.child("NormalFont"); normal != nil {
			attrs["inherits"] = normal.attrDefault("style", "")
		}
	}
	return &xmlNode{name: "FontString", attrs: attrs, children: append([]*xmlNode(nil), group.children...)}
}

// applyWidgetAttrs reads the merged widget element attributes.
func (l *Loader) applyWidgetAttrs(w *widget, node *xmlNode) {
	if id, ok := node.attr("id"); ok {
		if n, err := strconv.Atoi(id); err == nil {
			w.id = n
		}
	}
	if v, ok := node.attr("text"); ok {
		w.text = v
	}
	if parentName, ok := node.attr("parent"); ok && parentName != "" {
		if p := l.rt.lookup(parentName); p != nil {
			w.parent = p
			addWidgetChild(p, w)
		}
	}
	if v, ok := node.attr("hidden"); ok {
		w.shown = !parseBool(v, false)
	}
	if v, ok := node.attr("toplevel"); ok {
		w.topLevel = parseBool(v, false)
	}
	if v, ok := node.attr("movable"); ok {
		w.movable = parseBool(v, false)
	}
	if v, ok := node.attr("enableMouse"); ok {
		w.enableMouse = parseBool(v, false)
	}
	if v, ok := node.attr("enableKeyboard"); ok {
		w.enableKeyboard = parseBool(v, false)
	}
	if v, ok := node.attr("clampedToScreen"); ok {
		w.clampedToScreen = parseBool(v, false)
	}
	if v, ok := node.attr("frameStrata"); ok {
		w.frameStrata = frameStrataOrder(v)
	}
	if v, ok := node.attr("frameLevel"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			w.frameLevel = n
		}
	}
	if v, ok := node.attr("scale"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			w.scale = f
		}
	}
	if v, ok := node.attr("alpha"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			w.alpha = f
		}
	}
	if v, ok := node.attr("setAllPoints"); ok && parseBool(v, false) {
		w.points = []anchorPoint{{point: "TOPLEFT", relativePoint: "TOPLEFT"}, {point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"}}
	}
	if v, ok := node.attr("setAllPoints"); (ok && parseBool(v, false)) || (w.width == 0 && w.height == 0 && len(w.points) == 0 && w.kind == kindTexture) {
		w.points = []anchorPoint{
			{point: "TOPLEFT", relativePoint: "TOPLEFT"},
			{point: "BOTTOMRIGHT", relativePoint: "BOTTOMRIGHT"},
		}
	}
	switch w.kind {
	case kindModel, kindModelFFX:
		if v, ok := node.attr("file"); ok {
			w.modelFile = v
		}
		if v, ok := node.attr("fogNear"); ok {
			w.fogNear, _ = strconv.ParseFloat(v, 64)
			w.hasFog = true
		}
		if v, ok := node.attr("fogFar"); ok {
			w.fogFar, _ = strconv.ParseFloat(v, 64)
			w.hasFog = true
		}
	case kindEditBox:
		if v, ok := node.attr("letters"); ok {
			w.maxLetters, _ = strconv.Atoi(v)
		}
		if v, ok := node.attr("bytes"); ok {
			w.maxBytes, _ = strconv.Atoi(v)
		}
		if v, ok := node.attr("historyLines"); ok {
			w.historyLines, _ = strconv.Atoi(v)
		}
		if v, ok := node.attr("password"); ok {
			w.password = parseBool(v, false)
		}
		if v, ok := node.attr("autoFocus"); ok {
			w.autoFocus = parseBool(v, false)
		}
	case kindSlider:
		if v, ok := node.attr("orientation"); ok {
			w.orientation = v
		}
		if v, ok := node.attr("valueStep"); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				w.valueStep = f
			}
		}
		if mm := node.child("MinMaxValues"); mm != nil {
			if av := mm.child("AbsValue"); av != nil {
				w.minValue = attrFloat(av, "min", 0)
				w.maxValue = attrFloat(av, "max", 0)
			}
		}
		if d := node.child("DefaultValue"); d != nil {
			if av := d.child("AbsValue"); av != nil {
				w.value = attrFloat(av, "val", 0)
			}
		}
	case kindScrollingMessageFrame:
		if v, ok := node.attr("maxLines"); ok {
			w.messageMaxLines, _ = strconv.Atoi(v)
		}
		if v, ok := node.attr("displayDuration"); ok {
			w.messageDuration, _ = strconv.ParseFloat(v, 64)
		}
	}
}

// compileScript loads an XML script body as a function with the implicit
// parameter list the client compiles script handlers with. The chunk name
// carries the file and handler so error positions attribute correctly.
// namedHandler returns a function value that dispatches to the named
// global at call time, matching the function="..." script attribute.
func (l *Loader) namedHandler(fnName string) *lua.LFunction {
	return l.rt.L.NewFunction(func(L *lua.LState) int {
		fn := L.GetGlobal(fnName)
		if fn.Type() != lua.LTFunction {
			return 0
		}
		n := L.GetTop()
		L.Push(fn)
		for i := 1; i <= n; i++ {
			L.Push(L.Get(i))
		}
		L.Call(n, lua.MultRet)
		return 0
	})
}

func (l *Loader) compileScript(body, handler, chunkName string) *lua.LFunction {
	L := l.rt.L
	top := L.GetTop()
	defer L.SetTop(top)
	source := "return function(" + scriptParams(handler) + ") " + body + "\nend"
	fn, err := L.LoadString(source)
	if err != nil {
		fmt.Printf("COMPILE DEBUG [%s]: %v\nSOURCE: %q\n", chunkName, err, source)
		l.rt.recordScriptError(strings.TrimPrefix(chunkName, "@"), err.Error())
		return nil
	}
	L.Push(fn)
	if err := L.PCall(0, 1, nil); err != nil {
		fmt.Printf("PCALL DEBUG [%s]: %v\nSOURCE: %q\n", chunkName, err, source)
		l.rt.recordScriptError(strings.TrimPrefix(chunkName, "@"), err.Error())
		return nil
	}
	value := L.Get(-1)
	L.Pop(1)
	result, ok := value.(*lua.LFunction)
	if !ok {
		return nil
	}
	return result
}

func parseAnchors(node *xmlNode, parentName string) []anchorPoint {
	var points []anchorPoint
	for _, a := range node.children {
		if a.name != "Anchor" {
			continue
		}
		p := anchorPoint{point: a.attrDefault("point", "CENTER")}
		if v, ok := a.attr("relativeTo"); ok {
			p.relativeTo = resolveParentName(v, parentName)
		}
		if v, ok := a.attr("relativePoint"); ok {
			p.relativePoint = v
		}
		p.x = attrFloat(a, "x", 0)
		p.y = attrFloat(a, "y", 0)
		if off := a.child("Offset"); off != nil {
			p.x = attrFloat(off, "x", p.x)
			p.y = attrFloat(off, "y", p.y)
			if d := off.child("AbsDimension"); d != nil {
				p.x = attrFloat(d, "x", p.x)
				p.y = attrFloat(d, "y", p.y)
			}
		}
		points = append(points, p)
	}
	return points
}

func parseBackdrop(node *xmlNode) *backdrop {
	bd := &backdrop{}
	bd.bgFile = node.attrDefault("bgFile", "")
	bd.edgeFile = node.attrDefault("edgeFile", "")
	bd.tile = parseBool(node.attrDefault("tile", "false"), false)
	if ts := node.child("TileSize"); ts != nil {
		bd.tileSize = backdropValue(ts)
	}
	if es := node.child("EdgeSize"); es != nil {
		bd.edgeSize = backdropValue(es)
	}
	if ins := node.child("BackgroundInsets"); ins != nil {
		if ai := ins.child("AbsInset"); ai != nil {
			bd.insetL = attrFloat(ai, "left", 0)
			bd.insetR = attrFloat(ai, "right", 0)
			bd.insetT = attrFloat(ai, "top", 0)
			bd.insetB = attrFloat(ai, "bottom", 0)
		}
	}
	for _, c := range node.children {
		if c.name == "Color" {
			bd.bgColor = rgba{attrFloat(c, "r", 0), attrFloat(c, "g", 0), attrFloat(c, "b", 0), attrFloat(c, "a", 1)}
		}
		if c.name == "EdgeColor" {
			bd.edgeColor = rgba{attrFloat(c, "r", 0), attrFloat(c, "g", 0), attrFloat(c, "b", 0), attrFloat(c, "a", 1)}
		}
	}
	return bd
}

func backdropValue(node *xmlNode) float64 {
	if node == nil {
		return 0
	}
	if value := node.child("AbsValue"); value != nil {
		return attrFloat(value, "val", 0)
	}
	return attrFloat(node, "val", 0)
}

func attrFloat(n *xmlNode, key string, def float64) float64 {
	if v, ok := n.attr(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(v) {
	case "true", "1":
		return true
	case "false", "0":
		return false
	}
	return def
}

// resolveRelative resolves a file attribute against the including file's
// directory, the rule the client uses for Include and Script elements.
func resolveRelative(includingFile, file string) string {
	joined := dirOf(includingFile) + file
	if strings.Contains(joined, "..") {
		return normalizePath(joined)
	}
	return joined
}

func dirOf(path string) string {
	path = strings.ReplaceAll(path, "/", "\\")
	if idx := strings.LastIndex(path, "\\"); idx >= 0 {
		return path[:idx+1]
	}
	return ""
}

// mergeTemplate overlays an instance definition onto its template:
// instance attributes win, Size/Anchors/Backdrop from the instance replace
// the template's groups, all other template children (regions, nested
// frames, scripts) are kept ahead of the instance's, and instance script
// handlers replace same-named template handlers.
func mergeTemplate(l *Loader, tpl, instance *xmlNode) *xmlNode {
	if inherits := tpl.attrDefault("inherits", ""); inherits != "" {
		for _, name := range inheritanceNames(inherits) {
			if parent, ok := l.rt.virtuals[name]; ok && parent != tpl {
				tpl = mergeTemplate(l, parent, tpl)
			}
		}
	}
	merged := &xmlNode{name: instance.name, attrs: make(map[string]string)}
	for k, v := range tpl.attrs {
		merged.attrs[k] = v
	}
	for k, v := range instance.attrs {
		merged.attrs[k] = v
	}
	replaced := map[string]bool{}
	for _, group := range []string{"Size", "Anchors", "Backdrop", "TexCoords", "Color", "ButtonText", "NormalFont", "HighlightFont", "DisabledFont", "CheckedFont", "NormalTexture", "PushedTexture", "DisabledTexture", "HighlightTexture", "CheckedTexture", "DisabledCheckedTexture", "ThumbTexture", "TextInsets", "HitRectInsets"} {
		if instance.child(group) != nil {
			replaced[group] = true
		}
	}
	for _, c := range tpl.children {
		if !replaced[c.name] {
			merged.children = append(merged.children, c)
		}
	}
	merged.children = append(merged.children, instance.children...)
	return merged
}

func inheritanceNames(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func removeHandler(scripts *xmlNode, handler string) {
	kept := scripts.children[:0]
	for _, c := range scripts.children {
		if c.name != handler {
			kept = append(kept, c)
		}
	}
	scripts.children = kept
}
