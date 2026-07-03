package gui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/alphonse927/kpixiv/internal/platform"
)

type trackScroll struct {
	container.Scroll
	atBottom bool
}

func (s *trackScroll) Scrolled(ev *fyne.ScrollEvent) {
	s.Scroll.Scrolled(ev)
	if ev.Scrolled.DY < 0 {
		s.atBottom = false
	} else if ev.Scrolled.DY > 0 {
		s.atBottom = true
	}
}

type journalViewer struct {
	window     fyne.Window
	label      *widget.Label
	scroll     *trackScroll
	cancel     context.CancelFunc
	firstBatch bool
}

func (ui *settingsUI) showLogViewer() {
	ctx, cancel := context.WithCancel(context.Background())

	label := widget.NewLabel("Loading logs...")
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Monospace: true}

	scroll := &trackScroll{atBottom: true}
	scroll.Content = label
	scroll.Direction = container.ScrollVerticalOnly
	scroll.ExtendBaseWidget(scroll)

	bg := canvas.NewRectangle(color.RGBA{R: 25, G: 30, B: 40, A: 255})

	content := container.NewStack(
		bg,
		container.NewPadded(scroll),
	)

	v := &journalViewer{
		label:      label,
		scroll:     scroll,
		cancel:     cancel,
		firstBatch: false,
	}

	w := guiApp.NewWindow("KPixiv Logs")
	w.Resize(fyne.NewSize(900, 600))
	w.SetContent(content)

	w.SetCloseIntercept(func() {
		cancel()
		w.Close()
	})

	v.window = w
	w.Show()

	go v.readJournal(ctx)
}

func (v *journalViewer) readJournal(ctx context.Context) {
	lines, err := platform.ReadJournal(ctx, "kpixiv.service")
	if err != nil {
		fyne.Do(func() {
			v.label.SetText(fmt.Sprintf("Unable to read the systemd journal.\n\n%s", err))
		})
		return
	}

	var buf []string
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return
			}
			buf = append(buf, line)
		case <-ticker.C:
			if len(buf) == 0 {
				continue
			}
			pending := buf
			buf = nil

			fyne.Do(func() {
				v.appendLines(pending)
			})
		case <-ctx.Done():
			return
		}
	}
}

func (v *journalViewer) appendLines(lines []string) {
	if !v.firstBatch {
		v.label.SetText(strings.Join(lines, "\n"))
		v.firstBatch = true
		v.scroll.atBottom = true
		v.scroll.ScrollToBottom()
		time.AfterFunc(50*time.Millisecond, func() {
			fyne.Do(func() {
				v.scroll.ScrollToBottom()
			})
		})
		return
	}

	text := v.label.Text
	for _, l := range lines {
		text += "\n" + l
	}
	if strings.Count(text, "\n") > 1000 {
		text = trimLines(text, 500)
	}
	v.label.SetText(text)

	if v.scroll.atBottom {
		v.scroll.ScrollToBottom()
	}
}

func trimLines(s string, keep int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= keep {
		return s
	}
	return strings.Join(lines[len(lines)-keep:], "\n")
}
