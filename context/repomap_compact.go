package context

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// parseGoFileCompact extracts structural information in compact format
// This reduces token usage by 30-40% compared to the verbose format
func parseGoFileCompact(filePath, relPath string) (string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}
	
	var parts []string
	
	// Package (compact: "pkg agent" instead of "package agent")
	if node.Name != nil {
		parts = append(parts, fmt.Sprintf("pkg %s", node.Name.Name))
	}
	
	// Imports - only show third-party/project imports, skip stdlib
	var importantImports []string
	for _, imp := range node.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		// Only include if it contains a dot (non-stdlib) or is github.com
		if strings.Contains(path, ".") {
			importantImports = append(importantImports, path)
		}
	}
	if len(importantImports) > 0 {
		parts = append(parts, fmt.Sprintf("imports: %s", strings.Join(importantImports, ", ")))
	}
	
	// Functions and methods (compact signatures)
	for _, decl := range node.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			sig := compactFuncSignature(funcDecl)
			if sig != "" {
				parts = append(parts, sig)
			}
		}
	}
	
	// Types (compact format)
	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok {
			if genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						typeStr := compactTypeDecl(typeSpec)
						if typeStr != "" {
							parts = append(parts, typeStr)
						}
					}
				}
			}
		}
	}
	
	if len(parts) <= 1 {
		return "", nil // Empty or package-only file
	}
	
	return strings.Join(parts, "\n"), nil
}

// compactFuncSignature creates a compact function signature
// Format: "Receiver.Name(params) returns" or just "Name(params) returns"
func compactFuncSignature(decl *ast.FuncDecl) string {
	var sig strings.Builder
	
	// Receiver (method)
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recvType := compactType(decl.Recv.List[0].Type)
		sig.WriteString(recvType)
		sig.WriteString(".")
	}
	
	// Function name
	sig.WriteString(decl.Name.Name)
	
	// Parameters (types only, no names)
	sig.WriteString("(")
	if decl.Type.Params != nil {
		params := []string{}
		for _, param := range decl.Type.Params.List {
			pType := compactType(param.Type)
			count := len(param.Names)
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				params = append(params, pType)
			}
		}
		sig.WriteString(strings.Join(params, ", "))
	}
	sig.WriteString(")")
	
	// Return types
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		results := []string{}
		for _, result := range decl.Type.Results.List {
			rType := compactType(result.Type)
			results = append(results, rType)
		}
		if len(results) == 1 {
			sig.WriteString(" ")
			sig.WriteString(results[0])
		} else {
			sig.WriteString(" (")
			sig.WriteString(strings.Join(results, ", "))
			sig.WriteString(")")
		}
	}
	
	return sig.String()
}

// compactTypeDecl creates a compact type declaration
func compactTypeDecl(spec *ast.TypeSpec) string {
	typeName := spec.Name.Name
	
	switch t := spec.Type.(type) {
	case *ast.StructType:
		// For structs: count fields and show exported ones
		if t.Fields == nil || len(t.Fields.List) == 0 {
			return fmt.Sprintf("type %s struct{}", typeName)
		}
		
		exported := []string{}
		totalFields := 0
		for _, field := range t.Fields.List {
			if len(field.Names) > 0 {
				for _, name := range field.Names {
					totalFields++
					if isExported(name.Name) {
						fType := compactType(field.Type)
						exported = append(exported, fmt.Sprintf("%s %s", name.Name, fType))
					}
				}
			} else {
				// Embedded field
				totalFields++
				fType := compactType(field.Type)
				exported = append(exported, fType)
			}
		}
		
		// If few exported fields, show them; otherwise just count
		if len(exported) > 0 && len(exported) <= 3 {
			return fmt.Sprintf("type %s struct{%s}", typeName, strings.Join(exported, "; "))
		}
		return fmt.Sprintf("type %s struct{%d fields}", typeName, totalFields)
		
	case *ast.InterfaceType:
		// For interfaces: list method names
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return fmt.Sprintf("type %s interface{}", typeName)
		}
		
		methods := []string{}
		for _, method := range t.Methods.List {
			if len(method.Names) > 0 {
				methods = append(methods, method.Names[0].Name)
			} else {
				// Embedded interface
				methods = append(methods, compactType(method.Type))
			}
		}
		
		if len(methods) <= 5 {
			return fmt.Sprintf("type %s interface{%s}", typeName, strings.Join(methods, ", "))
		}
		return fmt.Sprintf("type %s interface{%d methods}", typeName, len(methods))
		
	default:
		// Aliases and other types
		typeStr := compactType(spec.Type)
		return fmt.Sprintf("type %s = %s", typeName, typeStr)
	}
}

// compactType converts an AST type expression to a compact string
func compactType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + compactType(t.X)
	case *ast.ArrayType:
		return "[]" + compactType(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", compactType(t.Key), compactType(t.Value))
	case *ast.SelectorExpr:
		pkg := compactType(t.X)
		// Omit package for common stdlib
		if isStdlib(pkg) {
			return t.Sel.Name
		}
		return pkg + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		return "chan"
	case *ast.Ellipsis:
		return "..." + compactType(t.Elt)
	default:
		return "?"
	}
}

// isExported returns true if name starts with uppercase
func isExported(name string) bool {
	if name == "" {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

// isStdlib checks if a package is likely stdlib (to omit in compact output)
func isStdlib(pkg string) bool {
	stdlibs := []string{
		"context", "fmt", "os", "io", "http", "time", "sync",
		"errors", "strings", "bytes", "encoding", "net", "path",
	}
	for _, s := range stdlibs {
		if pkg == s {
			return true
		}
	}
	return false
}
