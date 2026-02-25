package context

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RepoMapEntry represents a structural element in the codebase
type RepoMapEntry struct {
	FilePath string
	Content  string // Structural summary, not full file
	IsGoFile bool
}

// BuildRepoMap generates a structural summary of the codebase
// For Go files: function signatures, types, structs, interfaces
// For non-Go files: just filename and size
func BuildRepoMap(rootDir string, ignorePatterns []string) (string, error) {
	var entries []RepoMapEntry
	
	// Walk the directory
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			// Skip common directories to ignore
			baseName := filepath.Base(path)
			if baseName == ".git" || baseName == "node_modules" || 
			   baseName == "vendor" || baseName == ".openclaw" ||
			   baseName == "logs" {
				return filepath.SkipDir
			}
			return nil
		}
		
		// Skip ignored patterns
		relPath, _ := filepath.Rel(rootDir, path)
		if shouldIgnore(relPath, ignorePatterns) {
			return nil
		}
		
		// Process Go files with AST parsing
		if strings.HasSuffix(path, ".go") {
			summary, err := parseGoFile(path, relPath)
			if err != nil {
				// If parsing fails, still include the file name
				entries = append(entries, RepoMapEntry{
					FilePath: relPath,
					Content:  fmt.Sprintf("// Parse error: %v", err),
					IsGoFile: true,
				})
				return nil
			}
			if summary != "" {
				entries = append(entries, RepoMapEntry{
					FilePath: relPath,
					Content:  summary,
					IsGoFile: true,
				})
			}
		} else if isRelevantFile(path) {
			// For non-Go files, just show filename and size
			entries = append(entries, RepoMapEntry{
				FilePath: relPath,
				Content:  fmt.Sprintf("// %s (%d bytes)", filepath.Base(path), info.Size()),
				IsGoFile: false,
			})
		}
		
		return nil
	})
	
	if err != nil {
		return "", err
	}
	
	// Sort entries by path for consistency
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FilePath < entries[j].FilePath
	})
	
	// Build the final repo map string
	var builder strings.Builder
	builder.WriteString("# Repository Structure\n\n")
	
	for _, entry := range entries {
		builder.WriteString(fmt.Sprintf("## %s\n", entry.FilePath))
		builder.WriteString(entry.Content)
		builder.WriteString("\n\n")
	}
	
	return builder.String(), nil
}

// parseGoFile extracts structural information from a Go file using AST
func parseGoFile(filePath, relPath string) (string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}
	
	var builder strings.Builder
	
	// Package declaration
	if node.Name != nil {
		builder.WriteString(fmt.Sprintf("package %s\n\n", node.Name.Name))
	}
	
	// Collect imports
	if len(node.Imports) > 0 {
		builder.WriteString("imports:\n")
		for _, imp := range node.Imports {
			builder.WriteString(fmt.Sprintf("  %s\n", imp.Path.Value))
		}
		builder.WriteString("\n")
	}
	
	// Walk declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Function or method
			builder.WriteString(formatFuncDecl(d))
			
		case *ast.GenDecl:
			// Type, const, var declarations
			builder.WriteString(formatGenDecl(d))
		}
	}
	
	return builder.String(), nil
}

// formatFuncDecl formats a function declaration
func formatFuncDecl(decl *ast.FuncDecl) string {
	var builder strings.Builder
	
	// Receiver (for methods)
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		builder.WriteString("func (")
		builder.WriteString(formatFieldList(decl.Recv))
		builder.WriteString(") ")
	} else {
		builder.WriteString("func ")
	}
	
	// Function name
	builder.WriteString(decl.Name.Name)
	
	// Parameters
	builder.WriteString("(")
	if decl.Type.Params != nil {
		builder.WriteString(formatFieldList(decl.Type.Params))
	}
	builder.WriteString(")")
	
	// Results
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		builder.WriteString(" ")
		if len(decl.Type.Results.List) > 1 || len(decl.Type.Results.List[0].Names) > 1 {
			builder.WriteString("(")
			builder.WriteString(formatFieldList(decl.Type.Results))
			builder.WriteString(")")
		} else {
			builder.WriteString(formatFieldList(decl.Type.Results))
		}
	}
	
	builder.WriteString("\n")
	return builder.String()
}

// formatGenDecl formats type, const, var declarations
func formatGenDecl(decl *ast.GenDecl) string {
	var builder strings.Builder
	
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			builder.WriteString(formatTypeSpec(s))
			
		case *ast.ValueSpec:
			// const or var
			builder.WriteString(fmt.Sprintf("%s ", decl.Tok.String()))
			for i, name := range s.Names {
				if i > 0 {
					builder.WriteString(", ")
				}
				builder.WriteString(name.Name)
			}
			if s.Type != nil {
				builder.WriteString(" ")
				builder.WriteString(exprToString(s.Type))
			}
			builder.WriteString("\n")
		}
	}
	
	return builder.String()
}

// formatTypeSpec formats a type specification
func formatTypeSpec(spec *ast.TypeSpec) string {
	var builder strings.Builder
	builder.WriteString("type ")
	builder.WriteString(spec.Name.Name)
	builder.WriteString(" ")
	
	switch t := spec.Type.(type) {
	case *ast.StructType:
		builder.WriteString("struct {\n")
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				builder.WriteString("  ")
				if len(field.Names) > 0 {
					for i, name := range field.Names {
						if i > 0 {
							builder.WriteString(", ")
						}
						builder.WriteString(name.Name)
					}
					builder.WriteString(" ")
				}
				builder.WriteString(exprToString(field.Type))
				if field.Tag != nil {
					builder.WriteString(" ")
					builder.WriteString(field.Tag.Value)
				}
				builder.WriteString("\n")
			}
		}
		builder.WriteString("}\n")
		
	case *ast.InterfaceType:
		builder.WriteString("interface {\n")
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				builder.WriteString("  ")
				if len(method.Names) > 0 {
					builder.WriteString(method.Names[0].Name)
				}
				builder.WriteString(exprToString(method.Type))
				builder.WriteString("\n")
			}
		}
		builder.WriteString("}\n")
		
	default:
		builder.WriteString(exprToString(spec.Type))
		builder.WriteString("\n")
	}
	
	return builder.String()
}

// formatFieldList formats a field list (parameters, results, receivers)
func formatFieldList(fields *ast.FieldList) string {
	if fields == nil || len(fields.List) == 0 {
		return ""
	}
	
	var parts []string
	for _, field := range fields.List {
		typeStr := exprToString(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typeStr)
		} else {
			for _, name := range field.Names {
				parts = append(parts, fmt.Sprintf("%s %s", name.Name, typeStr))
			}
		}
	}
	
	return strings.Join(parts, ", ")
}

// exprToString converts an expression to a string representation
func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
		
	case *ast.StarExpr:
		return "*" + exprToString(e.X)
		
	case *ast.ArrayType:
		return "[]" + exprToString(e.Elt)
		
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", exprToString(e.Key), exprToString(e.Value))
		
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
		
	case *ast.FuncType:
		var builder strings.Builder
		builder.WriteString("func(")
		if e.Params != nil {
			builder.WriteString(formatFieldList(e.Params))
		}
		builder.WriteString(")")
		if e.Results != nil && len(e.Results.List) > 0 {
			builder.WriteString(" ")
			if len(e.Results.List) > 1 {
				builder.WriteString("(")
				builder.WriteString(formatFieldList(e.Results))
				builder.WriteString(")")
			} else {
				builder.WriteString(formatFieldList(e.Results))
			}
		}
		return builder.String()
		
	case *ast.InterfaceType:
		return "interface{}"
		
	case *ast.StructType:
		return "struct{...}"
		
	case *ast.ChanType:
		if e.Dir == ast.SEND {
			return "chan<- " + exprToString(e.Value)
		} else if e.Dir == ast.RECV {
			return "<-chan " + exprToString(e.Value)
		}
		return "chan " + exprToString(e.Value)
		
	case *ast.Ellipsis:
		return "..." + exprToString(e.Elt)
		
	default:
		return fmt.Sprintf("%T", e)
	}
}

// isRelevantFile checks if a non-Go file should be included in the repo map
func isRelevantFile(path string) bool {
	relevantExts := map[string]bool{
		".md":   true,
		".txt":  true,
		".yaml": true,
		".yml":  true,
		".json": true,
		".toml": true,
		".sh":   true,
		".sql":  true,
	}
	
	ext := strings.ToLower(filepath.Ext(path))
	return relevantExts[ext]
}

// shouldIgnore checks if a path should be ignored
func shouldIgnore(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// Also check if any part of the path matches
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}
