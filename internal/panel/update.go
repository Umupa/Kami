package panel

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Umupa/Kami/internal/design"
)

// A newer version of Kami takes the panel over as it opens
// (requirements F14): its version, a choice on the control bar, and its
// notes. The user's own session stays underneath and comes back with any
// choice but a restart.

// UpdatePhase is where a newer version is.
type UpdatePhase int

const (
	UpdateAvailable UpdatePhase = iota + 1
	UpdateDownloading
	// UpdateReady: installed, in effect once Kami restarts.
	UpdateReady
	UpdateFailed
)

// UpdateState is a newer version as the panel shows it.
type UpdateState struct {
	Phase            UpdatePhase
	Current, Version string
	// Notes are the version's release notes in Markdown.
	Notes string
	// Progress is the share downloaded, 0 to 1; below 0 when the size is
	// not known yet.
	Progress float64
}

// Updates is what the panel does about a newer version. Main thread.
type Updates struct {
	// Offer returns the update to show as the panel opens; false when there
	// is none, or the user put it off or skipped it.
	Offer func() (UpdateState, bool)
	// State returns the update as it is now, for the screen showing it.
	State func() (UpdateState, bool)
	// Install downloads the update and puts it in place; it does not
	// restart. Later puts the update off for this run; Skip for good.
	Install, Later, Skip func()
	Restart              func()
}

// updateChoice is an item of the update screen's control bar.
type updateChoice struct {
	label string
	do    func(p *Panel)
}

// ShowUpdate opens the update screen, as the menu bar's update item does,
// whether or not the user put it off.
func (p *Panel) ShowUpdate() {
	if p.deps.Updates.State == nil {
		return
	}
	if _, ok := p.deps.Updates.State(); ok {
		p.openUpdate()
	}
}

// offerUpdate opens the update screen as the panel shows, unless the user
// is setting Kami up.
func (p *Panel) offerUpdate() {
	if p.updating || p.deps.Updates.Offer == nil || p.unconfigured() || p.inSetup || p.welcoming() {
		return
	}
	if _, ok := p.deps.Updates.Offer(); ok {
		p.openUpdate()
	}
}

func (p *Panel) openUpdate() {
	p.updating, p.updateIdx, p.focusUpdate = true, 0, true
}

// closeUpdate gives the panel back to the user's session.
func (p *Panel) closeUpdate() {
	p.updating, p.updateConfirmed = false, false
	p.sourceIn.Focus()
}

// updateChoices are what the control bar offers in a phase. The first is
// the default, the second puts the update off.
func updateChoices(s UpdateState) []updateChoice {
	later := updateChoice{"稍后", func(p *Panel) {
		p.deps.Updates.Later()
		p.closeUpdate()
	}}
	skip := updateChoice{"跳过这个版本", func(p *Panel) {
		p.deps.Updates.Skip()
		p.closeUpdate()
	}}
	install := func(label string) updateChoice {
		return updateChoice{label, func(p *Panel) {
			// Confirmed once: the restart follows the download while the
			// panel stays up (updateView).
			p.updateConfirmed = true
			p.deps.Updates.Install()
		}}
	}
	switch s.Phase {
	case UpdateDownloading:
		label := "下载中…"
		if s.Progress >= 0 {
			label = fmt.Sprintf("下载中 %d%%", int(s.Progress*100))
		}
		return []updateChoice{{label, func(*Panel) {}}, later}
	case UpdateReady:
		return []updateChoice{{"立即重启", func(p *Panel) { p.updateConfirmed = true }}, later}
	case UpdateFailed:
		return []updateChoice{install("重试"), later, skip}
	}
	return []updateChoice{install("安装并重启"), later, skip}
}

// updateTitle is the first line of the update screen, and its muted end.
func updateTitle(s UpdateState) (string, string) {
	switch s.Phase {
	case UpdateReady:
		return fmt.Sprintf("Kami %s 已安装", s.Version), " · 重启后生效"
	case UpdateFailed:
		return fmt.Sprintf("Kami %s 安装失败", s.Version), " · 当前 " + s.Current + "，可以重试"
	}
	return fmt.Sprintf("Kami %s 可以安装", s.Version), " · 当前 " + s.Current
}

// updateView is the panel while it shows a newer version.
func (p *Panel) updateView(c *ui.Context, pal *design.Palette) {
	s, ok := p.deps.Updates.State()
	if !ok {
		p.closeUpdate()
		c.Invalidate()
		return
	}
	choices := updateChoices(s)
	p.updateIdx = min(p.updateIdx, len(choices)-1)

	title, rest := updateTitle(s)
	ui.Row(c.Key("update-title")).Shrink(0).PaddingX(design.SpaceX).PaddingY(design.SpaceSourceY).Children(func() {
		ui.Text(c, title).FontSize(p.sz.Source).TextColor(pal.Bright)
		ui.Text(c, rest).FontSize(p.sz.Source).TextColor(pal.Faint)
	})
	ui.Divider(c).Background(pal.Border)

	step := func(d int) { p.updateIdx = (p.updateIdx + d + len(choices)) % len(choices) }
	bar := ui.Row(c.Key("update-bar")).Height(design.ControlBarHeight).Shrink(0).PaddingX(design.SpaceX-8).Gap(2).Focusable().
		OnShortcut(0, ui.KeyTab, func() { step(1) }).
		OnShortcut(ui.Shift, ui.KeyTab, func() { step(-1) }).
		OnShortcut(0, ui.KeyEnter, func() { choices[p.updateIdx].do(p) }).
		// Escape puts it off, the second choice in every phase.
		OnShortcut(0, ui.KeyEscape, func() { choices[1].do(p) })
	bar.Children(func() {
		for i, ch := range choices {
			chip := ui.Box(c.Key("update-" + ch.label)).PaddingX(8).PaddingY(4).Radius(design.RadiusChip)
			chip.Children(func() {
				t := ui.Text(c, ch.label).FontSize(p.sz.Label)
				switch {
				case i == p.updateIdx:
					t.TextColor(pal.Accent).FontWeight(700)
				case chip.Hovered():
					t.TextColor(pal.Text)
				default:
					t.TextColor(pal.Muted)
				}
			})
			if i == p.updateIdx {
				chip.Background(pal.Raised)
			}
			if chip.Clicked() {
				p.updateIdx = i
				ch.do(p)
			}
		}
		ui.Spacer(c)
		p.kbd(c, pal, "tab", "切换")
	})
	if p.focusUpdate {
		bar.Focus()
		p.focusUpdate = false
	}
	ui.Divider(c).Background(pal.Border)

	ui.Scroll(c.Key("update-notes")).Grow(1).Shrink(1).MinHeight(0).Children(func() {
		ui.Column(c).PaddingX(design.SpaceX).PaddingY(14).Gap(8).Children(func() {
			ui.Text(c, "更新内容").FontSize(p.sz.Caption).TextColor(pal.Faint)
			if s.Notes == "" {
				ui.Text(c, "这个版本没有更新说明。").FontSize(p.sz.Body).TextColor(pal.Muted)
				return
			}
			p.updateMD.Theme = p.md.Theme
			markdown.View(c, s.Notes, &p.updateMD)
		})
	})
	p.filler(c)

	switch {
	case s.Phase == UpdateDownloading:
		c.After(150 * time.Millisecond)
	case s.Phase == UpdateReady && p.updateConfirmed:
		// The user asked for the restart and is still here; a reply under
		// way finishes first.
		if p.busy() {
			c.After(200 * time.Millisecond)
		} else {
			p.updateConfirmed = false
			p.deps.Updates.Restart()
		}
	}
}
