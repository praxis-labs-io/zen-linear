package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// keyHandlers is the context each handler answers for. handleGlobalKey routes
// to exactly one of them, and keyContext mirrors that routing.
var keyHandlers = map[string]keyContext{
	"handleGlobalKey":       keyContextGlobal,
	"handleNavigationKey":   keyContextNavigation,
	"handleNavSearchKey":    keyContextNavSearch,
	"handleIssuesKey":       keyContextIssues,
	"handleIssuesTableRune": keyContextIssues,
	"handleDetailsKey":      keyContextDetails,
	"handleCommentKey":      keyContextComment,
	"handleDetailsEditKey":  keyContextEditMode,
	"handleChooserKey":      keyContextChooser,
	"handleFieldEditorKey":  keyContextFieldEditor,
	"handleDescriptionKey":  keyContextDescription,
	"handleComposeKey":      keyContextWriting,
	"handlePaletteKey":      keyContextPalette,
}

// keyTokens is how a tcell key name reads in the reference.
var keyTokens = map[string]string{
	"KeyEnter":  "⏎",
	"KeyEscape": "Esc",
	"KeyCtrlC":  "⌃C",
	"KeyCtrlD":  "⌃D",
	"KeyCtrlU":  "⌃U",
	"KeyCtrlS":  "⌃S",
}

// keysNotListed are the cases the reference leaves out on purpose, each with
// the reason it is not something a reader needs told.
var keysNotListed = map[string]string{
	"KeyRune":       "the branch every rune case sits under, not a key",
	"KeyLeft":       "the arrows mirror the vim keys the rows already name",
	"KeyRight":      "the arrows mirror the vim keys the rows already name",
	"KeyUp":         "the arrows mirror the vim keys the rows already name",
	"KeyDown":       "the arrows mirror the vim keys the rows already name",
	"KeyTab":        "swallowed where a box has no second control",
	"KeyBacktab":    "swallowed where a box has no second control",
	"KeyBackspace":  "typing, which a reference does not name",
	"KeyBackspace2": "typing, which a reference does not name",
}

// A context that gains a key the reference does not name fails here rather
// than shipping as a key nobody can find.
func TestEveryHandlerKeyIsListed(t *testing.T) {
	app := newUXTestApp(t)

	listed := make(map[keyContext]map[string]bool, len(keySections))
	for _, section := range keySections {
		listed[section.context] = sectionKeys(app, section)
	}
	everywhere := listed[keyContextGlobal]

	found := 0
	for name, context := range keyHandlers {
		body, ok := handlerBody(t, name)
		if !ok {
			t.Errorf("no handler named %s: the table has drifted from the code", name)
			continue
		}
		for _, key := range handlerKeys(t, app, body) {
			found++
			// The legend shows the reader's own section and the global one, so a
			// key in either is a key they can find.
			if listed[context][key] || everywhere[key] {
				continue
			}
			t.Errorf("%s answers %q, which no section lists", name, key)
		}
	}
	if found == 0 {
		t.Fatal("read no keys out of any handler, the walk has drifted from the code")
	}
}

// sectionKeys is the keys a section names, one token per key. Matched exactly:
// against the rendered line, a bare "o" hits the "o" in "collapse a group".
func sectionKeys(app *App, section keySection) map[string]bool {
	keys := make(map[string]bool)
	for _, row := range section.rows(app) {
		if row.key == "" {
			continue
		}
		// A row can name a pair, as "j / k" or "{/}" does. Splitting leaves the
		// search key nothing, / being the whole of it.
		tokens := strings.FieldsFunc(row.key, func(r rune) bool { return r == '/' || r == ' ' })
		if len(tokens) == 0 {
			tokens = []string{row.key}
		}
		for _, token := range tokens {
			keys[token] = true
		}
	}
	return keys
}

// handlerBody finds one handler in the package sources.
func handlerBody(t *testing.T, name string) (*ast.BlockStmt, bool) {
	t.Helper()
	files, err := parser.ParseDir(token.NewFileSet(), ".", nil, 0)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}
	for _, pkg := range files {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Name.Name == name && fn.Body != nil {
					return fn.Body, true
				}
			}
		}
	}
	return nil, false
}

// handlerKeys is every key a handler switches on, as the reference would print
// it. A case the reference is not asked to cover is dropped.
func handlerKeys(t *testing.T, app *App, body *ast.BlockStmt) []string {
	t.Helper()
	keys := make([]string, 0, 16)
	ast.Inspect(body, func(node ast.Node) bool {
		stmt, ok := node.(*ast.SwitchStmt)
		if !ok || !switchesOnAKey(stmt) {
			return true
		}
		for _, item := range stmt.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range clause.List {
				if key, ok := caseKey(t, app, expr); ok {
					keys = append(keys, key)
				}
			}
		}
		return true
	})
	return keys
}

// switchesOnAKey reports whether a switch is over the event's key or rune,
// including the `switch r := event.Rune(); r` form the pane handlers use.
func switchesOnAKey(stmt *ast.SwitchStmt) bool {
	if stmt.Tag != nil && callsEventMethod(stmt.Tag) {
		return true
	}
	assign, ok := stmt.Init.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, value := range assign.Rhs {
		if callsEventMethod(value) {
			return true
		}
	}
	return false
}

func callsEventMethod(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == "event" && (selector.Sel.Name == "Key" || selector.Sel.Name == "Rune")
}

// caseKey turns one case expression into the key the reference would print,
// and reports false for a case it is not asked to cover.
func caseKey(t *testing.T, app *App, expr ast.Expr) (string, bool) {
	t.Helper()
	switch node := expr.(type) {
	case *ast.SelectorExpr:
		name := node.Sel.Name
		if _, skip := keysNotListed[name]; skip {
			return "", false
		}
		printed, known := keyTokens[name]
		if !known {
			t.Errorf("tcell.%s is neither in keyTokens nor keysNotListed", name)
			return "", false
		}
		return printed, true
	case *ast.BasicLit:
		return runeLiteral(t, node)
	case *ast.CallExpr:
		return resolvedKey(t, app, node)
	}
	return "", false
}

// runeLiteral is the key a `case 'j':` names.
func runeLiteral(t *testing.T, lit *ast.BasicLit) (string, bool) {
	t.Helper()
	if lit.Kind != token.CHAR {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		t.Errorf("unquoting %s: %v", lit.Value, err)
		return "", false
	}
	if value == " " {
		return "space", true
	}
	return value, true
}

// resolvedKey is the key a case reads back from the bindings. One the config
// moved reports where it moved to, and one it took reports nothing.
func resolvedKey(t *testing.T, app *App, call *ast.CallExpr) (string, bool) {
	t.Helper()
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	switch selector.Sel.Name {
	case "keysReferenceKey":
		return keyString(app.keysReferenceKey())
	case "actionKey":
		if len(call.Args) != 2 {
			return "", false
		}
		name, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return "", false
		}
		id, err := strconv.Unquote(name.Value)
		if err != nil {
			t.Errorf("unquoting the action id %s: %v", name.Value, err)
			return "", false
		}
		fallback, ok := call.Args[1].(*ast.BasicLit)
		if !ok {
			return "", false
		}
		key, ok := runeLiteral(t, fallback)
		if !ok {
			return "", false
		}
		return keyString(app.actionKey(id, []rune(key)[0]))
	}
	return "", false
}

// keyString drops a key the bindings answered 0 for, which is a key nothing
// answers and so nothing should list.
func keyString(key rune) (string, bool) {
	if key == 0 {
		return "", false
	}
	return string(key), true
}
