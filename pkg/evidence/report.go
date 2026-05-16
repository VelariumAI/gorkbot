package evidence

// Report is a compact wrapper for forward-compatible audit exports.
type Report struct {
	Receipt Receipt `json:"receipt"`
}

func (r Report) Normalized() Report {
	out := r
	out.Receipt = out.Receipt.Normalized()
	return out
}
