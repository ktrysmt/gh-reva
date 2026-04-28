package diff

func RenderSplit(files []*FileDiff, width int) string {
	return RenderUnified(files, width)
}
