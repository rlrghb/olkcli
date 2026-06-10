package graphapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// mutatingVerbs are the SDK request-builder methods that change server state.
var mutatingVerbs = map[string]bool{"Post": true, "Patch": true, "Put": true, "Delete": true}

// readOnlyMutationException lists *Client methods that call a "mutating" verb
// but are actually read-only (e.g. a POST query). Add to this set ONLY with a
// justification comment — never to silence a genuinely-mutating method.
var readOnlyMutationException = map[string]bool{
	"FindMeetingTimes": true, // POST /me/findMeetingTimes is a read-only query
}

// TestEveryMutationIsGuarded is a security invariant: every *Client method that
// performs a mutating Graph call (Post/Patch/Put/Delete) must call
// ensureWritable or ensureMaySend, so the --no-write / --no-send guarantee
// cannot silently regress when a new mutation is added. This caught three
// unguarded methods (MarkMessage, CopyDriveItem, ToggleChecklistItem).
func TestEveryMutationIsGuarded(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !isClientMethod(fn) {
				continue
			}
			var mutates, guards bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch {
				case mutatingVerbs[sel.Sel.Name]:
					mutates = true
				case sel.Sel.Name == "ensureWritable" || sel.Sel.Name == "ensureMaySend":
					guards = true
				}
				return true
			})
			if mutates && !guards && !readOnlyMutationException[fn.Name.Name] {
				t.Errorf("%s (%s) performs a mutating Graph call but never calls ensureWritable/ensureMaySend — "+
					"add the guard, or add it to readOnlyMutationException with justification", fn.Name.Name, file)
			}
		}
	}
}

func isClientMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "Client"
}

// proseFreeText is the set of result-struct json field names that carry
// externally-authored prose (subjects, names, bodies, locations, …) and so MUST
// be wrapped under --wrap-untrusted to defend an agent against prompt injection.
var proseFreeText = map[string]bool{
	"subject": true, "from": true, "to": true, "sender": true, "bodyPreview": true,
	"body": true, "displayName": true, "givenName": true, "surname": true,
	"middleName": true, "nickName": true, "name": true, "title": true,
	"location": true, "organizer": true, "attendees": true, "companyName": true,
	"jobTitle": true, "department": true, "officeLocation": true, "profession": true,
	"manager": true, "assistantName": true, "personalNotes": true, "spouseName": true,
	"children": true, "businessHomePage": true, "parentPath": true, "createdBy": true,
	"modifiedBy": true, "owner": true,
}

var jsonNameRe = regexp.MustCompile(`json:"([^",]+)`)

// TestProseFieldsAreUntrusted is a security invariant: every exported
// string/[]string struct field in the graphapi package whose json name is a
// known prose free-text name must carry `untrusted:"true"`. This prevents a new
// result field (e.g. a new attachment/sender/subject) from reaching an agent
// outside the [UNTRUSTED] markers. Input structs have no json tags, so they're
// naturally excluded.
func TestProseFieldsAreUntrusted(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				if field.Tag == nil || !isStringish(field.Type) {
					continue
				}
				tag := field.Tag.Value
				m := jsonNameRe.FindStringSubmatch(tag)
				if len(m) < 2 || !proseFreeText[m[1]] {
					continue
				}
				if !strings.Contains(reflect.StructTag(strings.Trim(tag, "`")).Get("untrusted"), "true") {
					name := "?"
					if len(field.Names) > 0 {
						name = field.Names[0].Name
					}
					t.Errorf("%s: field %s (json:%q) is prose free-text but lacks untrusted:\"true\" — "+
						"tag it so it's wrapped for agents", file, name, m[1])
				}
			}
			return true
		})
	}
}

func isStringish(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "string"
	case *ast.ArrayType:
		id, ok := e.Elt.(*ast.Ident)
		return ok && id.Name == "string"
	}
	return false
}
