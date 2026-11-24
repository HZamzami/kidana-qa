package domain

// Change is one field's old and new value in the change history.
type Change struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Diff returns the fields whose value changed, for the audit log.
func Diff(before, after map[string]string) map[string]any {
	out := map[string]any{}
	for k, b := range before {
		if a, ok := after[k]; ok && a != b {
			out[k] = Change{From: b, To: a}
		}
	}
	for k, a := range after {
		if _, ok := before[k]; !ok && a != "" {
			out[k] = Change{To: a}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
