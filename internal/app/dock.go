package app

import (
	"encoding/json"
	"strconv"

	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/config"
)

const showDockLabel, hideDockLabel = "显示 Dock 图标", "隐藏 Dock 图标"

func (a *app) dockMenuItem() *mygo.MenuItem {
	return &mygo.MenuItem{
		ID: "show-in-dock", Label: showDockLabel,
		Click: func(*mygo.MenuItem, *mygo.Window) {
			show := !a.store.Snapshot().Config.ShowInDock
			_, err := a.applyOps([]config.Op{{Path: "showInDock", Value: json.RawMessage(strconv.FormatBool(show))}})
			if err != nil {
				a.log.Warn("save Dock visibility", "err", err)
			}
			a.applyDock(a.store.Snapshot().Config)
		},
	}
}

// applyDock runs on the main thread after Ready, so MyGo's startup activation
// has already passed while the app was still an accessory. Changing visibility
// does not focus the app or open the panel; only a Dock click does that.
func (a *app) applyDock(c *config.Config) {
	label := showDockLabel
	if c.ShowInDock {
		label = hideDockLabel
	}
	a.menu.ItemByID("show-in-dock").SetLabel(label)
	if a.dockVisible == c.ShowInDock {
		return
	}
	policy := mygo.ActivationPolicyAccessory
	if c.ShowInDock {
		policy = mygo.ActivationPolicyRegular
	}
	a.setActivationPolicy(policy)
	a.dockVisible = c.ShowInDock
}
