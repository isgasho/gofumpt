// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"sort"
)

func gofumpt(fset *token.FileSet, file *ast.File) {
	f := &fumpter{
		fset:    fset,
		file:    fset.File(file.Pos()),
		astFile: file,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			f.stack = f.stack[:len(f.stack)-1]
			return true
		}
		f.visit(node)
		f.stack = append(f.stack, node)
		return true
	})
}

type fumpter struct {
	fset *token.FileSet
	file *token.File

	astFile *ast.File

	stack []ast.Node
}

func (f *fumpter) posLine(pos token.Pos) int {
	return f.file.Position(pos).Line
}

func (f *fumpter) commentsBetween(p1, p2 token.Pos) []*ast.CommentGroup {
	comments := f.astFile.Comments
	i1 := sort.Search(len(comments), func(i int) bool {
		return comments[i].Pos() >= p1
	})
	comments = comments[i1:]
	i2 := sort.Search(len(comments), func(i int) bool {
		return comments[i].Pos() >= p2
	})
	comments = comments[:i2]
	return comments
}

// addNewline is a hack to let us force a newline at a certain position.
func (f *fumpter) addNewline(at token.Pos, plus int) {
	offset := f.file.Position(at).Offset + plus

	field := reflect.ValueOf(f.file).Elem().FieldByName("lines")
	n := field.Len()
	lines := make([]int, 0, n+1)
	for i := 0; i < n; i++ {
		prev := int(field.Index(i).Int())
		if offset >= 0 && offset < prev {
			lines = append(lines, offset)
			offset = -1
		}
		lines = append(lines, prev)
	}
	if offset >= 0 {
		lines = append(lines, offset)
	}
	if !f.file.SetLines(lines) {
		panic(fmt.Sprintf("could not set lines to %v", lines))
	}
}

// removeLines joins all lines between two positions, for example to
// remove empty lines.
func (f *fumpter) removeLines(from, to token.Pos) {
	fromLine := f.posLine(from)
	toLine := f.posLine(to)
	for fromLine+1 < toLine {
		f.file.MergeLine(fromLine)
		toLine--
	}
}

func (f *fumpter) visit(node ast.Node) {
	switch node := node.(type) {
	case *ast.BlockStmt:
		comments := f.commentsBetween(node.Lbrace, node.Rbrace)
		if len(comments) > 0 {
			// for now, skip this case.
			break
		}
		if len(node.List) == 0 {
			f.removeLines(node.Lbrace, node.Rbrace)
			break
		}

		isFuncBody := false
		switch f.stack[len(f.stack)-1].(type) {
		case *ast.FuncDecl:
			isFuncBody = true
		case *ast.FuncLit:
			isFuncBody = true
		}

		if len(node.List) > 1 && !isFuncBody {
			// only if we have a single statement, or if
			// it's a func body.
			break
		}
		first := node.List[0]
		last := node.List[len(node.List)-1]

		f.removeLines(node.Lbrace, first.Pos())
		f.removeLines(last.End(), node.Rbrace)
	case *ast.CompositeLit:
		if len(node.Elts) == 0 {
			// doesn't have elements
			break
		}
		openLine := f.posLine(node.Lbrace)
		closeLine := f.posLine(node.Rbrace)
		if openLine == closeLine {
			// all in a single line
			break
		}

		newlineBetweenElems := false
		lastLine := openLine
		for _, elem := range node.Elts {
			if f.posLine(elem.Pos()) > lastLine {
				newlineBetweenElems = true
			}
			lastLine = f.posLine(elem.End())
		}
		if closeLine > lastLine {
			newlineBetweenElems = true
		}

		if !newlineBetweenElems {
			// no newlines between elements (and braces)
			break
		}

		first := node.Elts[0]
		if openLine == f.posLine(first.Pos()) {
			// We want the newline right after the brace.
			f.addNewline(node.Lbrace, 1)
			closeLine = f.posLine(node.Rbrace)
		}
		last := node.Elts[len(node.Elts)-1]
		if closeLine == f.posLine(last.End()) {
			f.addNewline(last.End(), 0)
		}
	}
}
