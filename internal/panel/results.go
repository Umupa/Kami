package panel

import (
	"context"

	"github.com/Umupa/Kami/internal/session"
)

// result is a mode's session put aside while another mode shows: its reply
// keeps streaming, and switching back shows it as it was left.
type result struct {
	sess   *session.Session
	list   conversationList
	cancel context.CancelFunc
}

// showResult shows the session of mode id, putting the one shown aside. An
// operator session the source still asks for stays: it belongs to no mode.
func (p *Panel) showResult(id string) {
	if p.sess != nil && (p.sess.Operator && p.operatorActive() || !p.sess.Operator && p.sess.Mode.ID == id) {
		return
	}
	p.park()
	r, ok := p.results[id]
	if !ok {
		return
	}
	delete(p.results, id)
	p.sess, p.list, p.cancel = r.sess, r.list, r.cancel
}

// park puts the session shown aside under its mode, leaving none shown. An
// operator session is not kept: it is stopped and dropped.
func (p *Panel) park() {
	if p.sess == nil {
		return
	}
	if p.sess.Operator {
		p.stop()
	} else {
		p.dropResult(p.sess.Mode.ID)
		if p.results == nil {
			p.results = map[string]result{}
		}
		p.results[p.sess.Mode.ID] = result{p.sess, p.list, p.cancel}
	}
	p.sess, p.list, p.cancel = nil, newList(), nil
}

// dropResult stops and forgets the session put aside for mode id.
func (p *Panel) dropResult(id string) {
	if r, ok := p.results[id]; ok {
		if r.cancel != nil {
			r.cancel()
		}
		delete(p.results, id)
	}
}

// clearResults stops and forgets every session, shown or put aside, as a
// new source does.
func (p *Panel) clearResults() {
	p.stop()
	for id := range p.results {
		p.dropResult(id)
	}
	p.sess, p.list = nil, newList()
}

// busy reports whether any session, shown or put aside, is streaming.
func (p *Panel) busy() bool {
	if p.sess != nil && p.sess.Busy() {
		return true
	}
	for _, r := range p.results {
		if r.sess.Busy() {
			return true
		}
	}
	return false
}
