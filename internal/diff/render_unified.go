package diff

func RenderUnified(files []*FileDiff, width int) string {
	out := ""
	for _, f := range files {
		out += "--- " + f.OrigName + "\n"
		out += "+++ " + f.NewName + "\n"
		for _, h := range f.Hunks {
			out += string(h.Body) + "\n"
		}
	}
	return out
}
