//go:build windows

package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"noco-path-opener/internal/actions"
)

const (
	windowTitle     = "文件操作"
	closeDelay      = 650 * time.Millisecond
	windowWidth     = 460
	windowHeight    = 250
	selectionHeight = 340
)

type Runner struct {
	mu sync.Mutex
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, req actions.Request, controller actions.Controller) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	view := newActionWindow(ctx, req, controller)
	return view.run()
}

type actionWindow struct {
	ctx        context.Context
	req        actions.Request
	controller actions.Controller

	mw *walk.MainWindow

	openButton          *walk.PushButton
	selectButton        *walk.PushButton
	cancelButton        *walk.PushButton
	confirmButton       *walk.PushButton
	reselectButton      *walk.PushButton
	confirmCancelButton *walk.PushButton

	actionView  *walk.Composite
	confirmView *walk.Composite

	instructionLabel *walk.Label
	statusLabel      *walk.Label
	fieldLabel       *walk.Label
	selectedPathEdit *walk.TextEdit

	selectedPath  string
	busy          bool
	closing       bool
	internalClose bool
}

func newActionWindow(ctx context.Context, req actions.Request, controller actions.Controller) *actionWindow {
	return &actionWindow{
		ctx:        ctx,
		req:        req,
		controller: controller,
	}
}

func (w *actionWindow) run() error {
	bounds := boundsNearCursor(windowWidth, windowHeight)

	mainWindow := MainWindow{
		AssignTo: &w.mw,
		Title:    windowTitle,
		Bounds:   Rectangle{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height},
		MinSize:  Size{Width: 420, Height: 220},
		Layout:   VBox{Margins: Margins{Left: 14, Top: 14, Right: 14, Bottom: 14}, Spacing: 10},
		OnDropFiles: func(files []string) {
			w.handleDropFiles(files)
		},
		Children: []Widget{
			Composite{
				AssignTo: &w.actionView,
				Layout:   VBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					Label{
						AssignTo: &w.instructionLabel,
						Text:     "请选择要执行的文件操作，也可以拖放文件或目录到此窗口。",
					},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							PushButton{
								AssignTo:  &w.openButton,
								Text:      "打开",
								OnClicked: w.openCurrent,
							},
							PushButton{
								AssignTo:  &w.selectButton,
								Text:      "上传或更新",
								OnClicked: w.choosePath,
							},
							HSpacer{},
							PushButton{
								AssignTo: &w.cancelButton,
								Text:     "取消",
								OnClicked: func() {
									w.mw.Close()
								},
							},
						},
					},
				},
			},
			Composite{
				AssignTo: &w.confirmView,
				Visible:  false,
				Layout:   VBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Label{
						AssignTo: &w.fieldLabel,
						Text:     "",
					},
					TextEdit{
						AssignTo: &w.selectedPathEdit,
						ReadOnly: true,
						VScroll:  true,
						Text:     "",
						MinSize:  Size{Width: 380, Height: 92},
					},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							PushButton{
								AssignTo:  &w.confirmButton,
								Text:      "确认更新",
								OnClicked: w.confirmUpdate,
							},
							PushButton{
								AssignTo:  &w.reselectButton,
								Text:      "重新选择",
								OnClicked: w.choosePath,
							},
							HSpacer{},
							PushButton{
								AssignTo: &w.confirmCancelButton,
								Text:     "取消",
								OnClicked: func() {
									w.mw.Close()
								},
							},
						},
					},
				},
			},
			Label{
				AssignTo: &w.statusLabel,
				Text:     "",
			},
			VSpacer{},
		},
	}
	if err := mainWindow.Create(); err != nil {
		return err
	}

	w.mw.Closing().Attach(w.handleClosing)
	w.mw.Run()

	return nil
}

func (w *actionWindow) openCurrent() {
	w.runAsync("正在打开...", func(ctx context.Context) error {
		return w.controller.OpenCurrent(ctx)
	}, "已打开。")
}

func (w *actionWindow) choosePath() {
	if w.locked() {
		return
	}

	useDir, ok := w.askSelectionKind()
	if !ok {
		return
	}

	var selected string
	var err error
	if useDir {
		selected, err = w.browseDirectory()
	} else {
		selected, err = w.browseFile()
	}
	if err != nil {
		w.showError(err)
		return
	}
	if strings.TrimSpace(selected) == "" {
		return
	}

	w.prepareSelection(selected)
}

func (w *actionWindow) handleDropFiles(files []string) {
	if w.locked() || len(files) == 0 {
		return
	}

	w.prepareSelection(files[0])
}

func (w *actionWindow) prepareSelection(path string) {
	prepared, err := w.controller.PreparePath(path)
	if err != nil {
		w.showError(err)
		return
	}

	w.selectedPath = prepared
	w.showConfirmView(prepared)
	w.setStatus("请确认要更新的路径。")
}

func (w *actionWindow) confirmUpdate() {
	selected := w.selectedPath
	if strings.TrimSpace(selected) == "" {
		w.showError(actions.ErrPathRequired)
		return
	}

	w.runAsync("正在更新...", func(ctx context.Context) error {
		return w.controller.UpdateSelected(ctx, selected)
	}, "已更新。")
}

func (w *actionWindow) runAsync(runningStatus string, fn func(context.Context) error, successStatus string) {
	if w.locked() {
		return
	}

	w.setBusy(true)
	w.setStatus(runningStatus)

	go func() {
		err := fn(w.ctx)
		w.mw.Synchronize(func() {
			if err != nil {
				w.setBusy(false)
				w.showError(err)
				return
			}

			w.setClosing()
			w.setStatus(successStatus)
			time.AfterFunc(closeDelay, func() {
				w.mw.Synchronize(func() {
					w.closeWindow()
				})
			})
		})
	}()
}

func (w *actionWindow) showConfirmView(path string) {
	if w.mw != nil {
		_ = w.mw.SetSize(walk.Size{Width: windowWidth, Height: selectionHeight})
	}
	w.actionView.SetVisible(false)
	w.confirmView.SetVisible(true)
	_ = w.fieldLabel.SetText(fmt.Sprintf("更新字段：%s", strings.TrimSpace(w.req.PathField)))
	_ = w.selectedPathEdit.SetText(path)
}

func (w *actionWindow) setBusy(busy bool) {
	w.busy = busy
	w.updateButtons()
}

func (w *actionWindow) setClosing() {
	w.busy = false
	w.closing = true
	w.updateButtons()
}

func (w *actionWindow) updateButtons() {
	enabled := !w.locked()
	for _, button := range []*walk.PushButton{
		w.openButton,
		w.selectButton,
		w.cancelButton,
		w.confirmButton,
		w.reselectButton,
		w.confirmCancelButton,
	} {
		if button != nil {
			button.SetEnabled(enabled)
		}
	}
}

func (w *actionWindow) locked() bool {
	return w.busy || w.closing
}

func (w *actionWindow) handleClosing(canceled *bool, _ walk.CloseReason) {
	if w.internalClose {
		return
	}
	if w.locked() {
		*canceled = true
	}
}

func (w *actionWindow) closeWindow() {
	w.internalClose = true
	defer func() {
		w.internalClose = false
	}()
	_ = w.mw.Close()
}

func (w *actionWindow) setStatus(status string) {
	if w.statusLabel != nil {
		_ = w.statusLabel.SetText(status)
	}
}

func (w *actionWindow) showError(err error) {
	w.setStatus(userMessage(err))
}

func (w *actionWindow) askSelectionKind() (useDir bool, ok bool) {
	var dlg *walk.Dialog
	var fileButton *walk.PushButton
	var dirButton *walk.PushButton
	var cancelButton *walk.PushButton
	var choice int

	bounds := boundsNearCursor(260, 130)
	if err := (Dialog{
		AssignTo:     &dlg,
		Title:        "上传或更新",
		FixedSize:    true,
		Size:         Size{Width: bounds.Width, Height: bounds.Height},
		CancelButton: &cancelButton,
		Layout:       VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 10},
		Children: []Widget{
			Label{Text: "请选择文件或目录。"},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					PushButton{
						AssignTo: &fileButton,
						Text:     "文件",
						OnClicked: func() {
							choice = 1
							dlg.Accept()
						},
					},
					PushButton{
						AssignTo: &dirButton,
						Text:     "目录",
						OnClicked: func() {
							choice = 2
							dlg.Accept()
						},
					},
					HSpacer{},
					PushButton{
						AssignTo: &cancelButton,
						Text:     "取消",
						OnClicked: func() {
							dlg.Cancel()
						},
					},
				},
			},
		},
	}).Create(w.mw); err != nil {
		w.showError(err)
		return false, false
	}

	if bounds.Width > 0 && bounds.Height > 0 {
		_ = dlg.SetBoundsPixels(bounds)
	}

	result := dlg.Run()
	if result != walk.DlgCmdOK || choice == 0 {
		return false, false
	}

	return choice == 2, true
}

func (w *actionWindow) browseFile() (string, error) {
	dlg := walk.FileDialog{
		Title:  "选择文件",
		Filter: "所有文件 (*.*)|*.*",
	}
	if initialDir := currentOrHomeDir(w.selectedPath); initialDir != "" {
		dlg.InitialDirPath = initialDir
	}

	accepted, err := dlg.ShowOpen(w.mw)
	if err != nil || !accepted {
		return "", err
	}

	return dlg.FilePath, nil
}

func (w *actionWindow) browseDirectory() (string, error) {
	dlg := walk.FileDialog{
		Title: "选择目录",
	}
	if initialDir := currentOrHomeDir(w.selectedPath); initialDir != "" {
		dlg.InitialDirPath = initialDir
	}

	accepted, err := dlg.ShowBrowseFolder(w.mw)
	if err != nil || !accepted {
		return "", err
	}

	return dlg.FilePath, nil
}

func userMessage(err error) string {
	switch {
	case errors.Is(err, actions.ErrCurrentPathRequired):
		return "当前路径为空。"
	case errors.Is(err, actions.ErrPathRequired):
		return "请选择文件或目录。"
	case errors.Is(err, actions.ErrPathNotAllowed):
		return "路径不在 allowed_roots 允许范围内。"
	case errors.Is(err, actions.ErrPathDoesNotExist):
		return "路径不存在。"
	case errors.Is(err, actions.ErrNocoDBConfigRequired):
		return "请先在 config.json 配置 nocodb_url 和 nocodb_token。"
	default:
		if err == nil {
			return ""
		}
		return err.Error()
	}
}

func currentOrHomeDir(path string) string {
	if strings.TrimSpace(path) != "" {
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return path
			}
			return filepath.Dir(path)
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	return ""
}

func boundsNearCursor(width, height int) walk.Rectangle {
	var point win.POINT
	if !win.GetCursorPos(&point) {
		return walk.Rectangle{X: 100, Y: 100, Width: width, Height: height}
	}

	x := int(point.X) + 16
	y := int(point.Y) + 16

	screenWidth := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	screenHeight := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	if screenWidth > 0 && x+width > screenWidth {
		x = screenWidth - width - 16
	}
	if screenHeight > 0 && y+height > screenHeight {
		y = screenHeight - height - 16
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return walk.Rectangle{X: x, Y: y, Width: width, Height: height}
}
