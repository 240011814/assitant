//go:build ignore

// 精确判定指定簇引用的、定义在簇外的 service 顶层符号。
// 用法: go run zz_cluster_deps.go "ai:ai_orchestration,orchestration,ai_agent,agent,skill,checkpoint,token_usage,prompt,agent_task" ...
package main

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

type spec struct {
	name     string
	prefixes []string
}

func main() {
	var specs []spec
	for _, a := range os.Args[1:] {
		parts := strings.SplitN(a, ":", 2)
		if len(parts) == 2 {
			specs = append(specs, spec{name: parts[0], prefixes: strings.Split(parts[1], ",")})
		} else {
			specs = append(specs, spec{name: parts[0], prefixes: []string{parts[0]}})
		}
	}
	fset := token.NewFileSet()
	pkgs, _ := parser.ParseDir(fset, "service", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
	}
	decl := map[string]string{}
	for fname, f := range pkg.Files {
		base := filepath.Base(fname)
		for _, d := range f.Decls {
			switch dd := d.(type) {
			case *ast.FuncDecl:
				if dd.Recv == nil || len(dd.Recv.List) == 0 {
					if _, ok := decl[dd.Name.Name]; !ok {
						decl[dd.Name.Name] = base
					}
				}
			case *ast.GenDecl:
				for _, s := range dd.Specs {
					switch sp := s.(type) {
					case *ast.TypeSpec:
						if _, ok := decl[sp.Name.Name]; !ok {
							decl[sp.Name.Name] = base
						}
					case *ast.ValueSpec:
						for _, n := range sp.Names {
							if _, ok := decl[n.Name]; !ok {
								decl[n.Name] = base
							}
						}
					}
				}
			}
		}
	}
	inSpec := func(base string, sp spec) bool {
		for _, p := range sp.prefixes {
			if strings.HasPrefix(base, p+"_") {
				return true
			}
		}
		return false
	}
	for _, sp := range specs {
		var myFiles []string
		for fname := range pkg.Files {
			if inSpec(filepath.Base(fname), sp) {
				myFiles = append(myFiles, fname)
			}
		}
		mySet := map[string]bool{}
		for _, f := range myFiles {
			mySet[filepath.Base(f)] = true
		}
		ref := map[string]bool{}
		for _, fname := range myFiles {
			ast.Inspect(pkg.Files[fname], func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					ref[id.Name] = true
				}
				return true
			})
		}
		byOwner := map[string][]string{}
		for s := range ref {
			owner, ok := decl[s]
			if !ok || mySet[owner] {
				continue
			}
			byOwner[strings.TrimSuffix(owner, ".go")] = append(byOwner[strings.TrimSuffix(owner, ".go")], s)
		}
		var owners []string
		for o := range byOwner {
			owners = append(owners, o)
		}
		sort.Strings(owners)
		fmt.Printf("[%s] %d files, 外部依赖 %d 个符号:\n", sp.name, len(myFiles), len(ref)-len(mySet))
		for _, o := range owners {
			sort.Strings(byOwner[o])
			fmt.Printf("  <- %s: %s\n", o, strings.Join(byOwner[o], ", "))
		}
		fmt.Println()
	}
}
