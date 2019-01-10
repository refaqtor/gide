// Copyright (c) 2018, The Gide Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gide

import (
	"image/color"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/goki/pi/filecat"
	"github.com/goki/pi/pi"

	"github.com/goki/gi/gi"
	"github.com/goki/gi/giv"
	"github.com/goki/gi/units"
	"github.com/goki/ki"
	"github.com/goki/ki/kit"
	"github.com/goki/pi/syms"
	"github.com/goki/pi/token"
)

// SymScope corresponds to the search scope
type SymbolsViewScope int

const (
	// SymScopeFile restricts the list of symbols to the active file
	SymScopeFile SymbolsViewScope = iota

	// SymScopePackage scopes list of symbols to the package of the active file
	SymScopePackage

	// SymScopeN is the number of symbol scopes
	SymScopeN
)

//go:generate stringer -type=SymbolsViewScope

var Kit_SymbolsViewScope = kit.Enums.AddEnumAltLower(SymScopeN, false, nil, "SymScope")

// MarshalJSON encodes
func (ev SymbolsViewScope) MarshalJSON() ([]byte, error) { return kit.EnumMarshalJSON(ev) }

// UnmarshalJSON decodes
func (ev *SymbolsViewScope) UnmarshalJSON(b []byte) error { return kit.EnumUnmarshalJSON(ev, b) }

// SymNode represents a language symbol -- the name of the node is
// the name of the symbol. Some symbols, e.g. type have children
type SymNode struct {
	ki.Node
	Symbol syms.Symbol `desc:"a string"`
	SRoot  *SymTree    `json:"-" xml:"-" desc:"root of the tree -- has global state"`
}

var KiT_SymNode = kit.Types.AddType(&SymNode{}, nil)

// SymbolsParams are parameters for structure view of file or package
type SymbolsParams struct {
	Scope SymbolsViewScope `desc:"scope of symbols to list"`
}

// SymbolsView is a widget that displays results of a file or package parse
type SymbolsView struct {
	gi.Layout
	Gide      Gide          `json:"-" xml:"-" desc:"parent gide project"`
	SymParams SymbolsParams `desc:"params for structure display"`
	SymTree   SymTree       `desc:"all the syms for the file or package in a tree"`
}

var KiT_SymbolsView = kit.Types.AddType(&SymbolsView{}, SymbolsViewProps)

// Params returns the symbols params
func (sv *SymbolsView) Params() *SymbolsParams {
	return &sv.Gide.ProjPrefs().Symbols
}

// SymbolsAction runs a new parse with current params
func (sv *SymbolsView) SymbolsAction() {
	sv.Gide.ProjPrefs().Symbols = sv.SymParams
	sv.Gide.Symbols()
}

// OpenSymbolsURL opens given symbols:/// url from Find
func (sv *SymbolsView) OpenSymbolsURL(ur string, ftv *giv.TextView) bool {
	ge := sv.Gide
	tv, reg, _, _, ok := ge.ParseOpenFindURL(ur, ftv)
	if !ok {
		return false
	}
	tv.UpdateStart()
	tv.Highlights = tv.Highlights[:0]
	tv.Highlights = append(tv.Highlights, reg)
	tv.UpdateEnd(true)
	tv.RefreshIfNeeded()
	tv.SetCursorShow(reg.Start)
	tv.GrabFocus()
	return true
}

//////////////////////////////////////////////////////////////////////////////////////
//    GUI config

// UpdateView updates view with current settings
func (sv *SymbolsView) UpdateView(ge Gide, sp SymbolsParams) {
	sv.Gide = ge
	sv.SymParams = sp
	_, updt := sv.StdSymbolsConfig()
	sv.ConfigToolbar()
	sb := sv.ScopeCombo()
	sb.SetCurIndex(int(sv.Params().Scope))
	sv.ConfigTree()
	sv.UpdateEnd(updt)
}

// StdConfig returns a TypeAndNameList for configuring a standard Frame
// -- can modify as desired before calling ConfigChildren on Frame using this
func (sv *SymbolsView) StdConfig() kit.TypeAndNameList {
	config := kit.TypeAndNameList{}
	config.Add(gi.KiT_ToolBar, "symbols-bar")
	config.Add(gi.KiT_Frame, "symbols-tree")
	return config
}

// StdSymbolsConfig configures a standard setup of the overall layout -- returns
// mods, updt from ConfigChildren and does NOT call UpdateEnd
func (sv *SymbolsView) StdSymbolsConfig() (mods, updt bool) {
	sv.Lay = gi.LayoutVert
	sv.SetProp("spacing", gi.StdDialogVSpaceUnits)
	config := sv.StdConfig()
	mods, updt = sv.ConfigChildren(config, false)
	return
}

// SymbolsBar returns the spell toolbar
func (sv *SymbolsView) SymbolsBar() *gi.ToolBar {
	tbi, ok := sv.ChildByName("symbols-bar", 0)
	if !ok {
		return nil
	}
	return tbi.(*gi.ToolBar)
}

// SymbolsBar returns the spell toolbar
func (sv *SymbolsView) SymbolsTree() *gi.Frame {
	tvi, ok := sv.ChildByName("symbols-tree", 0)
	if !ok {
		return nil
	}
	return tvi.(*gi.Frame)
}

// ScopeCombo returns the scope ComboBox
func (sv *SymbolsView) ScopeCombo() *gi.ComboBox {
	sb := sv.SymbolsBar()
	if sb == nil {
		return nil
	}
	scb, ok := sb.ChildByName("scope-combo", 5)
	if !ok {
		return nil
	}
	return scb.(*gi.ComboBox)
}

// ConfigToolbar adds toolbar.
func (sv *SymbolsView) ConfigToolbar() {
	svbar := sv.SymbolsBar()
	if svbar.HasChildren() {
		return
	}
	svbar.SetStretchMaxWidth()

	sl := svbar.AddNewChild(gi.KiT_Label, "scope-lbl").(*gi.Label)
	sl.SetText("Scope:")
	sl.Tooltip = "scope symbols to:"
	scb := svbar.AddNewChild(gi.KiT_ComboBox, "scope-combo").(*gi.ComboBox)
	scb.SetText("Scope")
	scb.Tooltip = sl.Tooltip
	scb.ItemsFromEnum(Kit_SymbolsViewScope, false, 0)
	scb.ComboSig.Connect(sv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		svv, _ := recv.Embed(KiT_SymbolsView).(*SymbolsView)
		smb := send.(*gi.ComboBox)
		eval := smb.CurVal.(kit.EnumValue)
		svv.Params().Scope = SymbolsViewScope(eval.Value)
	})
}

// ConfigTree adds a treeview to the symbolsview
func (sv *SymbolsView) ConfigTree() {
	if sv.SymTree.SRoot != nil {
		updt := sv.SymbolsTree().UpdateStart()
		sv.SymTree.DeleteChildren(true)
		if sv.SymParams.Scope == SymScopePackage {
			sv.SymTree.OpenPackageSymTree(sv)
		} else {
			sv.SymTree.OpenFileSymTree(sv)
		}
		sv.SymTree.TreeView.OpenAll()
		sv.SymbolsTree().UpdateEnd(updt)
		sv.GrabFocus()

		return
	}
	svtree := sv.SymbolsTree()
	svtree.SetStretchMaxWidth()
	svtree.SetStretchMaxHeight()
	if sv.SymParams.Scope == SymScopePackage {
		sv.SymTree.OpenPackageSymTree(sv)
	} else {
		sv.SymTree.OpenFileSymTree(sv)
	}
	svt := svtree.AddNewChild(KiT_SymbolTreeView, "symtree").(*SymbolTreeView)
	svt.SetRootNode(&sv.SymTree)
	sv.SymTree.TreeView = svt
	svt.TreeViewSig.Connect(sv.This(), func(recv, send ki.Ki, sig int64, data interface{}) {
		if data == nil {
			return
		}
		tvn, _ := data.(ki.Ki).Embed(KiT_SymbolTreeView).(*SymbolTreeView)
		//sve, _ := recv.Embed(KiT_SymbolsView).(*SymbolsView)
		if tvn.SrcNode.Ptr != nil {
			sn := tvn.SrcNode.Ptr.Embed(KiT_SymNode).(*SymNode)
			switch sig {
			case int64(giv.TreeViewSelected):
				sv.SelectSymbol(sn.Symbol)
				//sve.FileNodeSelected(fn, tvn)
				//case int64(giv.TreeViewOpened):
				//	sve.FileNodeOpened(fn, tvn)
				//case int64(giv.TreeViewClosed):
				//	sve.FileNodeClosed(fn, tvn)
			}
		}
	})
}

func (sv *SymbolsView) SelectSymbol(ssym syms.Symbol) {
	ge := sv.Gide
	tv := ge.ActiveTextView()
	if tv == nil {
		return
	}
	tv.UpdateStart()
	tv.Highlights = tv.Highlights[:0]
	tr := giv.NewTextRegion(ssym.SelectReg.St.Ln, ssym.SelectReg.St.Ch, ssym.SelectReg.Ed.Ln, ssym.SelectReg.Ed.Ch)
	tv.Highlights = append(tv.Highlights, tr)
	tv.UpdateEnd(true)
	tv.RefreshIfNeeded()
	tv.SetCursorShow(tr.Start)
	tv.GrabFocus()
}

// SymbolsViewProps are style properties for SymbolsView
var SymbolsViewProps = ki.Props{
	"background-color": &gi.Prefs.Colors.Background,
	"color":            &gi.Prefs.Colors.Font,
	"max-width":        -1,
	"max-height":       -1,
}

// SymTree is the root of a tree representing symbols of a package or file
type SymTree struct {
	SymNode
	NodeType reflect.Type `view:"-" json:"-" qxml:"-" desc:"type of node to create -- defaults to giv.FileNode but can use custom node types"`
	View     *SymbolsView
	TreeView *SymbolTreeView
}

var KiT_SymTree = kit.Types.AddType(&SymTree{}, SymTreeProps)

var SymTreeProps = ki.Props{}

// OpenTree opens a SymTree of symbols from a file or package parse
func (st *SymTree) OpenPackageSymTree(view *SymbolsView) {
	ge := view.Gide
	tv := ge.ActiveTextView()
	if tv == nil || tv.Buf == nil {
		return
	}
	st.SRoot = st // we are our own root..
	if st.NodeType == nil {
		st.NodeType = KiT_SymNode
	}
	st.SRoot.View = view

	path, _ := filepath.Split(string(tv.Buf.Filename))
	lp, _ := pi.LangSupport.Props(filecat.Go)
	pr := lp.Lang.Parser()
	pr.ReportErrs = true
	pkgsym := lp.Lang.ParseDir(path, pi.LangDirOpts{})
	if pkgsym != nil {
		syms.SaveSymDoc(pkgsym, filecat.Go, path)
	}

	gvars := []syms.Symbol{} // collect and list global vars first
	funcs := []syms.Symbol{} // collect and add functions (no receiver) to end
	for _, w := range pkgsym.Children {
		switch w.Kind {
		case token.NameFunction:
			funcs = append(funcs, *w)
		case token.NameVarGlobal:
			gvars = append(gvars, *w)
		case token.NameStruct, token.NameMap, token.NameArray, token.NameType, token.NameEnum:
			kid := st.AddNewChild(nil, w.Name)
			kn := kid.Embed(KiT_SymNode).(*SymNode)
			kn.SRoot = st.SRoot
			kn.Symbol = *w
			var methods []syms.Symbol
			var fields []syms.Symbol
			for _, x := range w.Children {
				if x.Kind == token.NameMethod {
					methods = append(methods, *x)
				} else if x.Kind == token.NameField {
					fields = append(fields, *x)
				}
			}
			sort.Slice(fields, func(i, j int) bool {
				return fields[i].Name < fields[j].Name
			})
			sort.Slice(methods, func(i, j int) bool {
				return methods[i].Name < methods[j].Name
			})
			for i, _ := range fields {
				dnm := fields[i].Name + ": " + fields[i].Type
				skid := kid.AddNewChild(nil, dnm)
				kn := skid.Embed(KiT_SymNode).(*SymNode)
				kn.SRoot = st.SRoot
				kn.Symbol = fields[i]
			}
			for i, _ := range methods {
				dnm := methods[i].Name
				idx := strings.Index(methods[i].Detail, "(")
				if idx > -1 {
					dnm = dnm + methods[i].Detail[idx-1:]
				} else {
					dnm = dnm + methods[i].Detail
				}
				skid := kid.AddNewChild(nil, dnm)
				kn := skid.Embed(KiT_SymNode).(*SymNode)
				kn.SRoot = st.SRoot
				kn.Symbol = methods[i]
			}
		case token.NameVar, token.NameVarClass:
			kid := st.AddNewChild(nil, w.Name)
			kn := kid.Embed(KiT_SymNode).(*SymNode)
			kn.SRoot = st.SRoot
			kn.Symbol = *w
			var temp []syms.Symbol
			for _, x := range w.Children {
				temp = append(temp, *x)
			}
			sort.Slice(temp, func(i, j int) bool {
				return temp[i].Name < temp[j].Name
			})
			for i, _ := range temp {
				dnm := temp[i].Name
				idx := strings.Index(temp[i].Detail, "(")
				if idx > -1 {
					dnm = dnm + temp[i].Detail[idx-1:]
				} else {
					dnm = dnm + temp[i].Detail
				}
				skid := kid.AddNewChild(nil, dnm)
				kn := skid.Embed(KiT_SymNode).(*SymNode)
				kn.SRoot = st.SRoot
				kn.Symbol = temp[i]
			}
		}
	}
	for i := range funcs {
		dnm := funcs[i].Name
		idx := strings.Index(funcs[i].Detail, "(")
		if idx > 0 {
			dnm = dnm + funcs[i].Detail[idx-1:]
		}
		skid := st.AddNewChild(nil, funcs[i].Name)
		kn := skid.Embed(KiT_SymNode).(*SymNode)
		kn.SRoot = st.SRoot
		kn.Symbol = funcs[i]
	}
	for i := range gvars {
		dnm := gvars[i].Name + ": " + gvars[i].Type
		skid := st.InsertNewChild(nil, 0, dnm)
		kn := skid.Embed(KiT_SymNode).(*SymNode)
		kn.SRoot = st.SRoot
		kn.Symbol = gvars[i]
	}
}

// OpenTree opens a SymTree of symbols from a file or package parse
func (st *SymTree) OpenFileSymTree(view *SymbolsView) {
	ge := view.Gide
	tv := ge.ActiveTextView()
	if tv == nil || tv.Buf == nil {
		return
	}
	fs := &tv.Buf.PiState // the parse info
	st.SRoot = st         // we are our own root..
	if st.NodeType == nil {
		st.NodeType = KiT_SymNode
	}
	st.SRoot.View = view

	gvars := []syms.Symbol{} // collect and list global vars first
	funcs := []syms.Symbol{} // collect and add functions (no receiver) to end
	for _, v := range fs.Syms {
		if v.Kind != token.NamePackage { // note: package symbol filename won't always corresp.
			continue
		}
		for _, w := range v.Children {
			switch w.Kind {
			case token.NameFunction:
				funcs = append(funcs, *w)
			case token.NameVarGlobal:
				gvars = append(gvars, *w)
			case token.NameStruct, token.NameMap, token.NameArray, token.NameType, token.NameEnum:
				kid := st.AddNewChild(nil, w.Name)
				kn := kid.Embed(KiT_SymNode).(*SymNode)
				kn.SRoot = st.SRoot
				kn.Symbol = *w
				var methods []syms.Symbol
				var fields []syms.Symbol
				for _, x := range w.Children {
					if x.Kind == token.NameMethod {
						methods = append(methods, *x)
					} else if x.Kind == token.NameField {
						fields = append(fields, *x)
					}
				}
				sort.Slice(fields, func(i, j int) bool {
					return fields[i].Name < fields[j].Name
				})
				sort.Slice(methods, func(i, j int) bool {
					return methods[i].Name < methods[j].Name
				})
				for i, _ := range fields {
					dnm := fields[i].Name + ": " + fields[i].Type
					skid := kid.AddNewChild(nil, dnm)
					kn := skid.Embed(KiT_SymNode).(*SymNode)
					kn.SRoot = st.SRoot
					kn.Symbol = fields[i]
				}
				for i, _ := range methods {
					dnm := methods[i].Name
					idx := strings.Index(methods[i].Detail, "(")
					if idx > -1 {
						dnm = dnm + methods[i].Detail[idx-1:]
					} else {
						dnm = dnm + methods[i].Detail
					}
					skid := kid.AddNewChild(nil, dnm)
					kn := skid.Embed(KiT_SymNode).(*SymNode)
					kn.SRoot = st.SRoot
					kn.Symbol = methods[i]
				}
			case token.NameVar, token.NameVarClass:
				kid := st.AddNewChild(nil, w.Name)
				kn := kid.Embed(KiT_SymNode).(*SymNode)
				kn.SRoot = st.SRoot
				kn.Symbol = *w
				var temp []syms.Symbol
				for _, x := range w.Children {
					//if x.Kind == token.NameMethod || x.Kind == token.NameVar {
					temp = append(temp, *x)
					//}
				}
				sort.Slice(temp, func(i, j int) bool {
					return temp[i].Name < temp[j].Name
				})
				for i, _ := range temp {
					dnm := temp[i].Name
					idx := strings.Index(temp[i].Detail, "(")
					if idx > -1 {
						dnm = dnm + temp[i].Detail[idx-1:]
					} else {
						dnm = dnm + temp[i].Detail
					}
					skid := kid.AddNewChild(nil, dnm)
					kn := skid.Embed(KiT_SymNode).(*SymNode)
					kn.SRoot = st.SRoot
					kn.Symbol = temp[i]
				}
			}
		}
	}
	for i := range funcs {
		dnm := funcs[i].Name
		idx := strings.Index(funcs[i].Detail, "(")
		if idx > 0 {
			dnm = dnm + funcs[i].Detail[idx-1:]
		}
		skid := st.AddNewChild(nil, funcs[i].Name)
		kn := skid.Embed(KiT_SymNode).(*SymNode)
		kn.SRoot = st.SRoot
		kn.Symbol = funcs[i]
	}
	for i := range gvars {
		dnm := gvars[i].Name + ": " + gvars[i].Type
		skid := st.InsertNewChild(nil, 0, dnm)
		kn := skid.Embed(KiT_SymNode).(*SymNode)
		kn.SRoot = st.SRoot
		kn.Symbol = gvars[i]
	}
}

// SymbolTreeView is a TreeView that knows how to operate on FileNode nodes
type SymbolTreeView struct {
	giv.TreeView
}

var KiT_SymbolTreeView = kit.Types.AddType(&SymbolTreeView{}, nil)

func init() {
	kit.Types.SetProps(KiT_SymbolTreeView, SymbolTreeViewProps)
}

// SymNode returns the SrcNode as a *gide* SymNode
func (st *SymbolTreeView) SymNode() *SymNode {
	sn := st.SrcNode.Ptr.Embed(KiT_SymNode)
	if sn == nil {
		return nil
	}
	return sn.(*SymNode)
}

var SymbolTreeViewProps = ki.Props{
	"indent":           units.NewValue(2, units.Ch),
	"spacing":          units.NewValue(.5, units.Ch),
	"border-width":     units.NewValue(0, units.Px),
	"border-radius":    units.NewValue(0, units.Px),
	"padding":          units.NewValue(0, units.Px),
	"margin":           units.NewValue(1, units.Px),
	"text-align":       gi.AlignLeft,
	"vertical-align":   gi.AlignTop,
	"color":            &gi.Prefs.Colors.Font,
	"background-color": "inherit",
	".exec": ki.Props{
		"font-weight": gi.WeightBold,
	},
	".open": ki.Props{
		"font-style": gi.FontItalic,
	},
	"#icon": ki.Props{
		"width":   units.NewValue(1, units.Em),
		"height":  units.NewValue(1, units.Em),
		"margin":  units.NewValue(0, units.Px),
		"padding": units.NewValue(0, units.Px),
		"fill":    &gi.Prefs.Colors.Icon,
		"stroke":  &gi.Prefs.Colors.Font,
	},
	"#branch": ki.Props{
		"icon":             "widget-wedge-down",
		"icon-off":         "widget-wedge-right",
		"margin":           units.NewValue(0, units.Px),
		"padding":          units.NewValue(0, units.Px),
		"background-color": color.Transparent,
		"max-width":        units.NewValue(.8, units.Em),
		"max-height":       units.NewValue(.8, units.Em),
	},
	"#space": ki.Props{
		"width": units.NewValue(.5, units.Em),
	},
	"#label": ki.Props{
		"margin":    units.NewValue(0, units.Px),
		"padding":   units.NewValue(0, units.Px),
		"min-width": units.NewValue(16, units.Ch),
	},
	"#menu": ki.Props{
		"indicator": "none",
	},
	giv.TreeViewSelectors[giv.TreeViewActive]: ki.Props{},
	giv.TreeViewSelectors[giv.TreeViewSel]: ki.Props{
		"background-color": &gi.Prefs.Colors.Select,
	},
	giv.TreeViewSelectors[giv.TreeViewFocus]: ki.Props{
		"background-color": &gi.Prefs.Colors.Control,
	},
	"CtxtMenuActive": ki.PropSlice{},
}

func (st *SymbolTreeView) Style2D() {
	sn := st.SymNode()
	st.Class = ""
	if sn != nil {
		if sn.Symbol.Kind == token.NameType {
			st.Icon = gi.IconName("type")
		} else if sn.Symbol.Kind == token.NameVar || sn.Symbol.Kind == token.NameVarGlobal {
			st.Icon = gi.IconName("var")
		} else if sn.Symbol.Kind == token.NameMethod {
			st.Icon = gi.IconName("method")
		} else if sn.Symbol.Kind == token.NameFunction {
			st.Icon = gi.IconName("function")
		} else if sn.Symbol.Kind == token.NameField {
			st.Icon = gi.IconName("field")
		}
	}
	st.StyleTreeView()
	st.LayData.SetFromStyle(&st.Sty.Layout) // also does reset
}