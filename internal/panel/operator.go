package panel

import (
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/mode"
	"github.com/Umupa/cida-next/internal/operator"
	"github.com/Umupa/cida-next/internal/session"
)

// Operator is what operator mode needs from the app.
type Operator struct {
	// State describes the app now.
	State func() operator.State
	// Apply applies edits to config.json and returns what changed; Undo
	// puts back the file as it was before the last Apply.
	Apply func(ops []config.Op) ([]config.Change, error)
	Undo  func() bool
	// Run runs an action a reply asked for.
	Run func(action string)
}

// operatorMode is the mode of an operator session: its prompt comes from
// operator.System, not from here.
var operatorMode = mode.Mode{ID: "operator", Name: "Operator", Input: config.InputInstruction, Output: config.OutputMarkdown}

// operatorActive reports whether the source asks for operator mode.
func (p *Panel) operatorActive() bool {
	_, ok := operator.Leading(p.source)
	return ok
}

// runOperator starts an operator session with the request after /operator.
func (p *Panel) runOperator(request string) {
	p.stop()
	p.notice = ""
	p.result = ui.ScrollState{}
	m := operatorMode
	m.Model = p.cfg.DefaultModel
	p.sess = session.New(m, request, m.Model)
	p.sess.Operator = true
	_, hasModel := p.cfg.Model(m.Model)
	if request == "" || !hasModel || p.deps.Operator.State == nil {
		p.answerLocally(p.sess.Last())
		return
	}
	p.sparklePending = true
	p.start()
}

// answerLocally answers without a model: the guide, which also tells how to
// configure one.
func (p *Panel) answerLocally(t *session.Turn) {
	var s operator.State
	if p.deps.Operator.State != nil {
		s = p.deps.Operator.State()
	}
	t.Reply, t.Model, t.Status, t.Local = operator.Guide(s), "", session.Done, true
}

// finishOperator applies what an operator reply asked for and says what
// changed at its end.
func (p *Panel) finishOperator(t *session.Turn) {
	plan, ok, err := operator.ParsePlan(t.Reply)
	t.Reply = operator.Visible(t.Reply)
	switch {
	case !ok:
		return
	case err != nil:
		t.Reply += "\n\n---\n\n没有修改：回复里的改动读不懂。"
		return
	}
	if len(plan.Ops) > 0 && p.deps.Operator.Apply != nil {
		changes, err := p.deps.Operator.Apply(plan.Ops)
		if err != nil {
			t.Reply += "\n\n---\n\n没有修改，因为改完的配置不合法：\n\n" + quoteLines(err.Error())
		} else {
			t.Reply += operator.ChangesText(changes)
			t.Undoable = len(changes) > 0
		}
	}
	for _, a := range plan.Actions {
		if p.deps.Operator.Run != nil {
			p.deps.Operator.Run(a)
		}
	}
}

// undoOperator puts back the configuration before the latest operator edit.
func (p *Panel) undoOperator() bool {
	if p.sess == nil || !p.sess.Last().Undoable || p.deps.Operator.Undo == nil {
		return false
	}
	t := p.sess.Last()
	if p.deps.Operator.Undo() {
		t.Undoable = false
		t.Reply += "\n\n已撤销，配置恢复到修改之前。"
	}
	return true
}

func quoteLines(s string) string {
	return "> " + strings.ReplaceAll(s, "\n", "\n> ")
}

var operatorIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"><path d="M6.2 6.2l7.3 2.6-3.1 1.1-1.1 3.1z"/><path d="M3 1.5v2M1.5 3h2M6.5 2.2l-.8.8M2.2 6.5l.8-.8"/></svg>`))

// operatorPill takes the place of the mode tabs while the source asks for
// operator mode; its × leaves the mode.
func (p *Panel) operatorPill(c *ui.Context, pal *design.Palette) {
	pill := ui.Row(c.Key("operator-pill")).Gap(5).PaddingX(8).PaddingY(4).Radius(design.RadiusChip).Background(pal.OperatorFill)
	pill.Children(func() {
		ui.Icon(c, operatorIcon).Size(14, 14).TextColor(pal.Operator)
		label := ui.Text(c, "Operator").FontSize(design.FontSecondary).FontWeight(600).TextColor(pal.Operator)
		if t := p.sheen(c); t < 1 {
			label.TextColor(sheenColor(pal.Operator, t, 0.5))
		}
		x := ui.Text(c, "×").FontSize(design.FontSecondary).TextColor(pal.Operator).Label("退出 Operator")
		if x.Clicked() {
			request, _ := operator.Leading(p.source)
			p.source = request
			p.sourceIn.Focus()
		}
	})
	ui.Text(c, "用一句话查看或修改设置").FontSize(design.FontCaption).TextColor(pal.TextMuted).MarginStart(8)
}

// sheen returns how far the sheen has crossed operator mode's name since the
// mode turned on, 0 to 1; 1 once it is done or motion is reduced.
func (p *Panel) sheen(c *ui.Context) float32 {
	if !p.operatorActive() {
		p.operatorSince = time.Time{}
		return 1
	}
	now := c.Now()
	if p.operatorSince.IsZero() {
		p.operatorSince = now
	}
	if c.Preferences().ReduceMotion {
		return 1
	}
	t := float32(now.Sub(p.operatorSince)) / float32(design.MotionSheen)
	if t < 1 {
		c.Invalidate()
	}
	return min(t, 1)
}

// sheenColor lightens col where a band of light crosses at t, for a text at
// position x of its length, both 0 to 1.
func sheenColor(col ui.Color, t, x float32) ui.Color {
	band := 1 - float32(math.Abs(float64(x-(1.2-1.4*t))))/0.25
	if band <= 0 {
		return col
	}
	return col.Mix(ui.Hex("#FFFFFF"), 0.75*band)
}

// commandRanges colors /operator in the source, with the sheen crossing it
// as the mode turns on.
func (p *Panel) commandRanges(c *ui.Context, pal *design.Palette) []ui.TextRange {
	start, end, ok := operator.Token(p.source)
	if !ok {
		return nil
	}
	t := p.sheen(c)
	ranges := make([]ui.TextRange, 0, end-start)
	for i := start; i < end; i++ {
		col := pal.Operator
		if t < 1 {
			col = sheenColor(col, t, float32(i-start)/float32(end-start))
		}
		ranges = append(ranges, ui.TextRange{Start: i, End: i + 1, Color: col, Weight: 600})
	}
	return ranges
}

// sparkle is one particle of the burst over an operator request.
type sparkle struct {
	x, size, delay, dur, drift float32
	star                       bool
}

func newSparkles() []sparkle {
	const n = 18
	s := make([]sparkle, n)
	for i := range s {
		star := i%3 != 2
		size := 2.5 + rand.Float32()*2
		if star {
			size = 7 + rand.Float32()*6
		}
		s[i] = sparkle{
			x:     min(0.96, max(0.04, (float32(i)+rand.Float32())/n)),
			size:  size,
			delay: 0.15 + rand.Float32()*0.95,
			dur:   1.2 + rand.Float32()*0.8,
			drift: (rand.Float32() - 0.5) * 28,
			star:  star,
		}
	}
	return s
}

// drawSparkles paints the burst rising through r, elapsed seconds after the
// request was sent.
func drawSparkles(p *ui.Painter, r ui.Rect, sparkles []sparkle, elapsed float32, col ui.Color) {
	p.Clip(r, 0, func() {
		for _, s := range sparkles {
			t := (elapsed - s.delay) / s.dur
			if t <= 0 || t >= 1 {
				continue
			}
			rise := 1 - (1-t)*(1-t)
			x := r.X + s.x*r.W + s.drift*t
			y := r.Y + r.H + s.size - rise*(r.H+2*s.size)
			alpha := min(1, t/0.18) * min(1, (1-t)/0.3)
			c := col.Alpha(alpha)
			if !s.star {
				p.FillPath(new(ui.Path).Circle(x, y, s.size/2), c)
				continue
			}
			h := s.size / 2 * (0.4 + 0.6*min(1, t/0.18))
			star := new(ui.Path).MoveTo(x, y-h).
				QuadTo(x, y, x+h, y).QuadTo(x, y, x, y+h).
				QuadTo(x, y, x-h, y).QuadTo(x, y, x, y-h).Close()
			p.FillPath(star, c)
		}
	})
}

// sparkle draws the burst over the result for a while after an operator
// request is sent, unless motion is reduced.
func (p *Panel) sparkle(c *ui.Context, result ui.Element, pal *design.Palette) {
	if !p.sparklePending && p.sparkleAt.IsZero() {
		return
	}
	if c.Preferences().ReduceMotion {
		p.sparklePending, p.sparkleAt = false, time.Time{}
		return
	}
	if p.sparklePending {
		p.sparklePending, p.sparkleAt, p.sparkles = false, c.Now(), newSparkles()
	}
	elapsed := c.Now().Sub(p.sparkleAt)
	if elapsed >= design.MotionSparkles {
		p.sparkleAt, p.sparkles = time.Time{}, nil
		return
	}
	sparkles, seconds, col := p.sparkles, float32(elapsed.Seconds()), pal.Sparkle
	result.DrawOver(func(pt *ui.Painter, r ui.Rect) { drawSparkles(pt, r, sparkles, seconds, col) })
	c.Invalidate()
}
