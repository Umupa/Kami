package panel

// traceLayout records geometry and timing only. KAMI_TRACE_LAYOUT=1 enables
// it for a reproduction; source text, replies and configuration stay out.
func (p *Panel) traceLayout(event string, attrs ...any) {
	if !p.traceLayoutEnabled {
		return
	}
	p.deps.Log.Info("panel layout", append([]any{"event", event}, attrs...)...)
}
