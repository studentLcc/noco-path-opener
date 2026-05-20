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
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"

	"noco-path-opener/internal/actions"
)

const (
	baseWindowTitle        = "文件操作"
	closeDelay             = 650 * time.Millisecond
	windowWidth            = 400
	windowHeight           = 150
	selectionHeight        = 240
	BIF_BROWSEINCLUDEFILES = 0x00004000
	BIF_NEWDIALOGSTYLE     = 0x00000040
	BFFM_INITIALIZED       = 1
	BFFM_SETSELECTIONW     = win.WM_USER + 103
)

var browseFolderCallbackPtr uintptr

func init() {
	browseFolderCallbackPtr = syscall.NewCallback(browseFolderCallback)
}

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, req actions.Request, controller actions.Controller) error {
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

	openButton     *walk.PushButton
	selectButton   *walk.PushButton
	syncButton     *walk.PushButton
	cancelButton   *walk.PushButton
	confirmButton  *walk.PushButton
	reselectButton *walk.PushButton

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
		Title:    windowTitleFor(w.req),
		Bounds:   Rectangle{X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height},
		MinSize:  Size{Width: 360, Height: 128},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
		OnDropFiles: func(files []string) {
			w.handleDropFiles(files)
		},
		Children: []Widget{
			Composite{
				AssignTo: &w.actionView,
				Layout:   VBox{MarginsZero: true, Spacing: 8},
				Children: []Widget{
					Label{
						AssignTo: &w.instructionLabel,
						Text:     "请选择操作，也可以拖放文件或目录到窗口中。",
					},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 6},
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
							PushButton{
								AssignTo:  &w.syncButton,
								Text:      "同步远端",
								Visible:   w.req.HasRemoteSync(),
								OnClicked: w.syncRemote,
							},
						},
					},
				},
			},
			Composite{
				AssignTo: &w.confirmView,
				Visible:  false,
				Layout:   VBox{MarginsZero: true, Spacing: 6},
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
						MinSize:  Size{Width: 360, Height: 64},
					},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 6},
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
						},
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Label{
						AssignTo: &w.statusLabel,
						Text:     "",
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
	}
	if err := mainWindow.Create(); err != nil {
		return err
	}
	if rowKey, ok := w.req.RowKey(); ok {
		unregister := actions.RegisterRowWindow(rowKey, func() {
			w.mw.Synchronize(func() {
				w.bringToForeground()
			})
		})
		defer unregister()
	}

	w.mw.Starting().Attach(func() {
		w.bringToForeground()
	})
	w.mw.Closing().Attach(w.handleClosing)
	w.bringToForeground()
	w.mw.Run()

	return nil
}

func windowTitleFor(req actions.Request) string {
	rowID := req.RowDisplayID()
	if rowID == "" {
		return baseWindowTitle
	}
	return fmt.Sprintf("%s - %s", baseWindowTitle, rowID)
}

func (w *actionWindow) openCurrent() {
	w.runAsync("正在打开...", func(ctx context.Context) error {
		return w.controller.OpenCurrent(ctx)
	}, func() {
		w.closeAfterSuccess("已打开。")
	})
}

func (w *actionWindow) choosePath() {
	if w.locked() {
		return
	}

	selected, err := w.browsePath()
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
	}, func() {
		w.selectedPath = ""
		w.showActionView()
		w.setStatus("已更新。")
	})
}

func (w *actionWindow) syncRemote() {
	w.runAsync("正在同步远端...", func(ctx context.Context) error {
		return w.controller.SyncRemote(ctx)
	}, func() {
		w.showActionView()
		w.setStatus("已同步远端字段。")
	})
}

func (w *actionWindow) runAsync(runningStatus string, fn func(context.Context) error, onSuccess func()) {
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

			w.setBusy(false)
			if onSuccess != nil {
				onSuccess()
			}
		})
	}()
}

func (w *actionWindow) closeAfterSuccess(status string) {
	w.setClosing()
	w.setStatus(status)
	time.AfterFunc(closeDelay, func() {
		w.mw.Synchronize(func() {
			w.closeWindow()
		})
	})
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

func (w *actionWindow) showActionView() {
	if w.mw != nil {
		_ = w.mw.SetSize(walk.Size{Width: windowWidth, Height: windowHeight})
	}
	w.confirmView.SetVisible(false)
	w.actionView.SetVisible(true)
	_ = w.fieldLabel.SetText("")
	_ = w.selectedPathEdit.SetText("")
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
		w.syncButton,
		w.cancelButton,
		w.confirmButton,
		w.reselectButton,
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

func (w *actionWindow) bringToForeground() {
	if w.mw == nil || w.mw.Handle() == 0 {
		return
	}

	hwnd := w.mw.Handle()
	_ = w.mw.BringToTop()
	_ = win.SetWindowPos(hwnd, win.HWND_TOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
	_ = win.SetWindowPos(hwnd, win.HWND_NOTOPMOST, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_SHOWWINDOW)
	_ = win.BringWindowToTop(hwnd)
	_ = win.SetForegroundWindow(hwnd)
	_ = w.mw.Activate()
}

func (w *actionWindow) browsePath() (string, error) {
	if hr := win.OleInitialize(); hr != win.S_OK && hr != win.S_FALSE {
		return "", fmt.Errorf("OleInitialize Error: %v", hr)
	}
	defer win.OleUninitialize()

	var dialogTitle = syscall.StringToUTF16Ptr("选择文件或目录")
	var displayName [win.MAX_PATH]uint16
	var initialSelection []uint16
	var initialPtr uintptr
	if initialDir := currentOrHomeDir(w.selectedPath); initialDir != "" {
		initialSelection = syscall.StringToUTF16(initialDir)
		initialPtr = uintptr(unsafe.Pointer(&initialSelection[0]))
	}

	bi := win.BROWSEINFO{
		HwndOwner:      w.mw.Handle(),
		PszDisplayName: &displayName[0],
		LpszTitle:      dialogTitle,
		UlFlags:        BIF_NEWDIALOGSTYLE | BIF_BROWSEINCLUDEFILES,
		Lpfn:           browseFolderCallbackPtr,
		LParam:         initialPtr,
	}

	pidl := win.SHBrowseForFolder(&bi)
	if pidl == 0 {
		runtime.KeepAlive(initialSelection)
		return "", nil
	}
	defer win.CoTaskMemFree(pidl)
	defer runtime.KeepAlive(initialSelection)

	return pathFromPIDL(pidl)
}

func browseFolderCallback(hwnd win.HWND, msg uint32, lp, wp uintptr) uintptr {
	if msg == BFFM_INITIALIZED && lp != 0 {
		win.SendMessage(hwnd, BFFM_SETSELECTIONW, 1, lp)
	}
	return 0
}

func pathFromPIDL(pidl uintptr) (string, error) {
	var buf [win.MAX_PATH]uint16
	if !win.SHGetPathFromIDList(pidl, &buf[0]) {
		return "", errors.New("failed to resolve selected path")
	}
	return syscall.UTF16ToString(buf[:]), nil
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
	case errors.Is(err, actions.ErrRemoteSyncTableMismatch):
		return "同步配置与当前表不匹配"
	case errors.Is(err, actions.ErrLocalLookupEmpty):
		return "本地查询字段为空"
	case errors.Is(err, actions.ErrRemoteRecordNotFound):
		return "远端未找到匹配记录"
	case errors.Is(err, actions.ErrRemoteRecordAmbiguous):
		return "远端找到多条匹配记录"
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
