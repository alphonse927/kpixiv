package gui

import "fyne.io/fyne/v2"

type fixedWidthLayout struct {
	width float32
}

func (f *fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) != 2 {
		return
	}

	sidebar := objects[0]
	content := objects[1]

	sidebar.Resize(fyne.NewSize(f.width, size.Height))
	sidebar.Move(fyne.NewPos(0, 0))

	content.Resize(fyne.NewSize(size.Width-f.width, size.Height))
	content.Move(fyne.NewPos(f.width, 0))
}

func (f *fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) != 2 {
		return fyne.NewSize(0, 0)
	}

	sidebarMin := objects[0].MinSize()
	contentMin := objects[1].MinSize()

	width := f.width + contentMin.Width
	height := fyne.Max(sidebarMin.Height, contentMin.Height)

	return fyne.NewSize(width, height)
}

func NewFixedWidthLayout(w float32) fyne.Layout {
	return &fixedWidthLayout{
		width: w,
	}
}
