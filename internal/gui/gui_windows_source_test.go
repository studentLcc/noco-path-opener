package gui

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestWindowsChoosePathUsesSinglePathPicker(t *testing.T) {
	source, file := parseWindowsGUISource(t)

	if findFunc(file, "askSelectionKind") != nil {
		t.Fatalf("askSelectionKind still exists; upload/update should not ask for file or directory first")
	}

	choosePath := findFunc(file, "choosePath")
	if choosePath == nil {
		t.Fatalf("choosePath function not found")
	}

	if !funcCalls(choosePath, "browsePath") {
		t.Fatalf("choosePath does not call browsePath")
	}
	for _, forbidden := range []string{"askSelectionKind", "browseFile", "browseDirectory"} {
		if funcCalls(choosePath, forbidden) {
			t.Fatalf("choosePath still calls %s", forbidden)
		}
	}

	if !strings.Contains(string(source), "BIF_BROWSEINCLUDEFILES") {
		t.Fatalf("path picker does not enable file selection in the folder browser")
	}
}

func TestWindowsGUIHasSeparateUpdateAndUploadActions(t *testing.T) {
	source, file := parseWindowsGUISource(t)

	for _, token := range []string{
		`Text:      "更新"`,
		"OnClicked: w.chooseUpdatePath",
		`Text:      "上传"`,
		"OnClicked: w.chooseUploadPaths",
		"actionModeUpload",
		"w.controller.UploadSelected",
		`Text:      "继续添加"`,
	} {
		if !strings.Contains(string(source), token) {
			t.Fatalf("GUI source does not contain %s", token)
		}
	}

	confirmSelection := findFunc(file, "confirmSelection")
	if confirmSelection == nil || !funcCalls(confirmSelection, "confirmUpload") || !funcCalls(confirmSelection, "confirmUpdate") {
		t.Fatal("confirmSelection must dispatch to both upload and update confirmations")
	}
}

func TestWindowsUploadDropPreservesMultiplePaths(t *testing.T) {
	_, file := parseWindowsGUISource(t)
	handleDropFiles := findFunc(file, "handleDropFiles")
	if handleDropFiles == nil {
		t.Fatal("handleDropFiles function not found")
	}
	source := formatNode(t, handleDropFiles)
	if !strings.Contains(source, "len(files) > 1") || !strings.Contains(source, "actionModeUpload") {
		t.Fatal("multiple dropped paths must enter upload mode")
	}
	if !funcCalls(handleDropFiles, "prepareSelection") {
		t.Fatal("handleDropFiles does not preserve paths for confirmation")
	}
}

func TestWindowsGUIUsesCompactWindowSizing(t *testing.T) {
	source, file := parseWindowsGUISource(t)

	if got := constIntValue(t, file, "windowHeight"); got > 180 {
		t.Fatalf("windowHeight = %d, want <= 180", got)
	}
	if got := constIntValue(t, file, "selectionHeight"); got > 260 {
		t.Fatalf("selectionHeight = %d, want <= 260", got)
	}
	if strings.Contains(string(source), "VSpacer{}") {
		t.Fatalf("window still contains VSpacer; compact GUI should not reserve extra blank space")
	}
}

func TestWindowsGUIUsesSingleCancelButton(t *testing.T) {
	_, file := parseWindowsGUISource(t)

	if got := countPushButtonsWithText(file, "取消"); got != 1 {
		t.Fatalf("cancel buttons = %d, want 1", got)
	}
}

func TestWindowsGUIUsesSingleRemoteSyncButton(t *testing.T) {
	_, file := parseWindowsGUISource(t)

	if got := countPushButtonsWithText(file, "同步远端"); got != 1 {
		t.Fatalf("remote sync buttons = %d, want 1", got)
	}
}

func TestWindowsGUIRemoteSyncButtonIsConditional(t *testing.T) {
	source, _ := parseWindowsGUISource(t)

	for _, token := range []string{
		"syncButton",
		"w.req.HasRemoteSync()",
		`Text:      "同步远端"`,
		"OnClicked: w.syncRemote",
	} {
		if !strings.Contains(string(source), token) {
			t.Fatalf("remote sync button source does not contain %s", token)
		}
	}
}

func TestWindowsConfirmUpdateReturnsToActionView(t *testing.T) {
	_, file := parseWindowsGUISource(t)

	confirmUpdate := findFunc(file, "confirmUpdate")
	if confirmUpdate == nil {
		t.Fatalf("confirmUpdate function not found")
	}
	if !funcCalls(confirmUpdate, "showActionView") {
		t.Fatalf("confirmUpdate does not return to the action view after a successful update")
	}
	if funcCalls(confirmUpdate, "setClosing") {
		t.Fatalf("confirmUpdate should not close the window after a successful update")
	}
}

func TestWindowsSyncRemoteKeepsActionWindowOpen(t *testing.T) {
	_, file := parseWindowsGUISource(t)

	syncRemote := findFunc(file, "syncRemote")
	if syncRemote == nil {
		t.Fatalf("syncRemote function not found")
	}

	syncRemoteSource := formatNode(t, syncRemote)
	for _, token := range []string{
		`"正在同步远端..."`,
		"w.controller.SyncRemote(ctx)",
		`"已同步远端字段。"`,
	} {
		if !strings.Contains(syncRemoteSource, token) {
			t.Fatalf("syncRemote source does not contain %s", token)
		}
	}
	if funcCalls(syncRemote, "closeAfterSuccess") || funcCalls(syncRemote, "setClosing") || funcCalls(syncRemote, "closeWindow") {
		t.Fatalf("syncRemote should not close the window after a successful sync")
	}
}

func TestWindowsUpdateButtonsIncludesRemoteSyncButton(t *testing.T) {
	_, file := parseWindowsGUISource(t)

	updateButtons := findFunc(file, "updateButtons")
	if updateButtons == nil {
		t.Fatalf("updateButtons function not found")
	}
	if !strings.Contains(formatNode(t, updateButtons), "w.syncButton") {
		t.Fatalf("updateButtons does not include w.syncButton")
	}
}

func TestWindowsGUIBringsWindowToForeground(t *testing.T) {
	source, file := parseWindowsGUISource(t)

	run := findFunc(file, "run")
	if run == nil {
		t.Fatalf("run function not found")
	}
	if !funcCalls(run, "bringToForeground") {
		t.Fatalf("run does not bring the created GUI window to the foreground")
	}
	for _, token := range []string{"SetForegroundWindow", "HWND_TOPMOST", "HWND_NOTOPMOST"} {
		if !strings.Contains(string(source), token) {
			t.Fatalf("foreground handling does not use %s", token)
		}
	}
}

func TestWindowsGUITitleUsesOnlyRowID(t *testing.T) {
	source, file := parseWindowsGUISource(t)

	run := findFunc(file, "run")
	if run == nil {
		t.Fatalf("run function not found")
	}
	if !strings.Contains(formatNode(t, run), "windowTitleFor(w.req)") {
		t.Fatalf("run does not build the window title from the request row identity")
	}
	if strings.Contains(string(source), "Title:    windowTitle,") {
		t.Fatalf("main window still uses the static window title")
	}
	if !strings.Contains(string(source), "req.RowDisplayID()") {
		t.Fatalf("window title does not use the request row ID")
	}
	if strings.Contains(string(source), "req.RowLabel()") {
		t.Fatalf("window title still uses the full row identity")
	}
}

func TestWindowsGUIRegistersWindowFocusForDuplicateRows(t *testing.T) {
	source, file := parseWindowsGUISource(t)

	run := findFunc(file, "run")
	if run == nil {
		t.Fatalf("run function not found")
	}
	runSource := formatNode(t, run)
	for _, token := range []string{"RegisterRowWindow", "w.req.RowKey()", "defer unregister()"} {
		if !strings.Contains(runSource, token) {
			t.Fatalf("run does not register row window focus with %s", token)
		}
	}
	if !strings.Contains(string(source), "w.mw.Synchronize") {
		t.Fatalf("row window focus should synchronize foreground work onto the GUI thread")
	}
}

func parseWindowsGUISource(t *testing.T) ([]byte, *ast.File) {
	t.Helper()

	source, err := os.ReadFile("gui_windows.go")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	file, err := parser.ParseFile(token.NewFileSet(), "gui_windows.go", source, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	return source, file
}

func constIntValue(t *testing.T, file *ast.File, name string) int {
	t.Helper()

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range valueSpec.Names {
				if ident.Name != name {
					continue
				}
				if i >= len(valueSpec.Values) {
					t.Fatalf("const %s has no explicit value", name)
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("const %s is %s, want integer literal", name, formatNode(t, valueSpec.Values[i]))
				}
				got, err := strconv.Atoi(lit.Value)
				if err != nil {
					t.Fatalf("const %s value %q is not an int: %v", name, lit.Value, err)
				}
				return got
			}
		}
	}
	t.Fatalf("const %s not found", name)
	return 0
}

func countPushButtonsWithText(file *ast.File, text string) int {
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		ident, ok := lit.Type.(*ast.Ident)
		if !ok || ident.Name != "PushButton" {
			return true
		}

		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Text" {
				continue
			}
			value, ok := kv.Value.(*ast.BasicLit)
			if !ok || value.Kind != token.STRING {
				continue
			}
			unquoted, err := strconv.Unquote(value.Value)
			if err == nil && unquoted == text {
				count++
			}
		}
		return true
	})
	return count
}

func formatNode(t *testing.T, node ast.Node) string {
	t.Helper()

	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), node); err != nil {
		t.Fatalf("printer.Fprint() error = %v", err)
	}
	return b.String()
}

func findFunc(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func funcCalls(fn *ast.FuncDecl, name string) bool {
	called := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.Ident:
			called = called || fun.Name == name
		case *ast.SelectorExpr:
			called = called || fun.Sel.Name == name
		}
		return true
	})
	return called
}
