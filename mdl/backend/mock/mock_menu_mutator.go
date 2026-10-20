// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func (m *MockBackend) OpenNavigationProfileForMutation(profileName string) (backend.NavigationProfileMutator, error) {
	if m.OpenNavigationProfileForMutationFunc != nil {
		return m.OpenNavigationProfileForMutationFunc(profileName)
	}
	return nil, fmt.Errorf("MockBackend.OpenNavigationProfileForMutation not configured")
}

func (m *MockBackend) OpenMenuDocumentForMutation(id model.ID) (backend.MenuMutator, error) {
	if m.OpenMenuDocumentForMutationFunc != nil {
		return m.OpenMenuDocumentForMutationFunc(id)
	}
	return nil, fmt.Errorf("MockBackend.OpenMenuDocumentForMutation not configured")
}

// MenuMutator is an in-memory backend.NavigationProfileMutator over a
// MenuTreeNode tree, for executor tests: each operation changes Tree the way
// the BSON splice changes the stored menu, and is logged in Calls. Saved says
// whether Save ran — a refused statement must leave it false.
type MenuMutator struct {
	Tree  []*backend.MenuTreeNode
	Calls []string
	Saved bool

	Home      types.NavHomePageSpec
	RoleHomes map[string]types.NavHomePageSpec
	LoginPage string
	NotFound  string
}

var _ backend.NavigationProfileMutator = (*MenuMutator)(nil)

func (m *MenuMutator) log(format string, args ...any) {
	m.Calls = append(m.Calls, fmt.Sprintf(format, args...))
}

func (m *MenuMutator) MenuTree() []*backend.MenuTreeNode { return m.Tree }

// level is the list holding the item at path's parent, with a setter.
func (m *MenuMutator) level(parent backend.MenuItemPath) (*[]*backend.MenuTreeNode, error) {
	if len(parent) == 0 {
		return &m.Tree, nil
	}
	n := backend.MenuNodeAt(m.Tree, parent)
	if n == nil {
		return nil, fmt.Errorf("no item at %v", parent)
	}
	return &n.Items, nil
}

func nodesOf(specs []types.NavMenuItemSpec) []*backend.MenuTreeNode {
	var out []*backend.MenuTreeNode
	for _, s := range specs {
		out = append(out, &backend.MenuTreeNode{
			Caption:  s.Caption,
			NoAction: s.Action == nil && s.Page == "" && s.Microflow == "" && !s.SignOut,
			Items:    nodesOf(s.Items),
		})
	}
	return out
}

func (m *MenuMutator) InsertMenuItems(parent backend.MenuItemPath, index int, items []types.NavMenuItemSpec) error {
	l, err := m.level(parent)
	if err != nil {
		return err
	}
	if index < 0 || index > len(*l) {
		return fmt.Errorf("index %d outside %d items", index, len(*l))
	}
	*l = append(append(append([]*backend.MenuTreeNode(nil), (*l)[:index]...), nodesOf(items)...), (*l)[index:]...)
	m.log("insert %v %d %s", parent, index, captions(items))
	return nil
}

func (m *MenuMutator) ReplaceMenuItem(path backend.MenuItemPath, items []types.NavMenuItemSpec) error {
	l, err := m.level(path.Parent())
	if err != nil {
		return err
	}
	i := path.Index()
	*l = append(append(append([]*backend.MenuTreeNode(nil), (*l)[:i]...), nodesOf(items)...), (*l)[i+1:]...)
	m.log("replace %v %s", path, captions(items))
	return nil
}

func (m *MenuMutator) MoveMenuItem(from, target backend.MenuItemPath, position backend.MenuPosition) error {
	moved := backend.MenuNodeAt(m.Tree, from)
	tnode := backend.MenuNodeAt(m.Tree, target)
	if moved == nil || tnode == nil {
		return fmt.Errorf("no item at %v or %v", from, target)
	}
	src, _ := m.level(from.Parent())
	i := from.Index()
	*src = append(append([]*backend.MenuTreeNode(nil), (*src)[:i]...), (*src)[i+1:]...)
	// find target again
	var place func(l *[]*backend.MenuTreeNode) bool
	place = func(l *[]*backend.MenuTreeNode) bool {
		for j, n := range *l {
			if n == tnode {
				switch position {
				case backend.MenuInto:
					n.Items = append(n.Items, moved)
				case backend.MenuBefore:
					*l = append(append(append([]*backend.MenuTreeNode(nil), (*l)[:j]...), moved), (*l)[j:]...)
				default:
					*l = append(append(append([]*backend.MenuTreeNode(nil), (*l)[:j+1]...), moved), (*l)[j+1:]...)
				}
				return true
			}
			if place(&n.Items) {
				return true
			}
		}
		return false
	}
	if !place(&m.Tree) {
		return fmt.Errorf("target lost")
	}
	m.log("move %v %s %v", from, position, target)
	return nil
}

func (m *MenuMutator) DropMenuItem(path backend.MenuItemPath) error {
	l, err := m.level(path.Parent())
	if err != nil {
		return err
	}
	i := path.Index()
	*l = append(append([]*backend.MenuTreeNode(nil), (*l)[:i]...), (*l)[i+1:]...)
	m.log("drop %v", path)
	return nil
}

func (m *MenuMutator) SetMenuItemCaption(path backend.MenuItemPath, caption string) error {
	n := backend.MenuNodeAt(m.Tree, path)
	if n == nil {
		return fmt.Errorf("no item at %v", path)
	}
	n.Caption = caption
	m.log("caption %v %q", path, caption)
	return nil
}

func (m *MenuMutator) SetMenuItemAction(path backend.MenuItemPath, item types.NavMenuItemSpec) error {
	n := backend.MenuNodeAt(m.Tree, path)
	if n == nil {
		return fmt.Errorf("no item at %v", path)
	}
	n.NoAction = item.Action == nil && item.Page == "" && item.Microflow == "" && !item.SignOut
	m.log("action %v %T", path, item.Action)
	return nil
}

func (m *MenuMutator) SetMenuItemIcon(path backend.MenuItemPath, item types.NavMenuItemSpec) error {
	m.log("icon %v %s%s", path, item.IconKind, item.Icon)
	return nil
}

func (m *MenuMutator) SetHomePage(home types.NavHomePageSpec) error {
	if home.ForRole == "" {
		m.Home = home
	} else {
		if m.RoleHomes == nil {
			m.RoleHomes = map[string]types.NavHomePageSpec{}
		}
		m.RoleHomes[home.ForRole] = home
	}
	m.log("home %s %s", home.Target, home.ForRole)
	return nil
}

func (m *MenuMutator) DropRoleHomePage(role string) error {
	if _, ok := m.RoleHomes[role]; !ok {
		return fmt.Errorf("role %s has no home page", role)
	}
	delete(m.RoleHomes, role)
	m.log("drop home %s", role)
	return nil
}

func (m *MenuMutator) SetLoginPage(page string) error {
	m.LoginPage = page
	m.log("login %q", page)
	return nil
}

func (m *MenuMutator) SetNotFoundPage(page string) error {
	m.NotFound = page
	m.log("notfound %q", page)
	return nil
}

func (m *MenuMutator) Save() error {
	m.Saved = true
	return nil
}

func captions(items []types.NavMenuItemSpec) string {
	var out []string
	for _, it := range items {
		out = append(out, "'"+it.Caption+"'")
	}
	return strings.Join(out, ",")
}
