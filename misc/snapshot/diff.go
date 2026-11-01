package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiffDirs compares two snapshot directories and returns a readable
// unified diff, empty when they are identical.
func DiffDirs(a, b string) (string, error) {
	fa, err := listFiles(a)
	if err != nil {
		return "", err
	}
	fb, err := listFiles(b)
	if err != nil {
		return "", err
	}
	all := map[string]bool{}
	for f := range fa {
		all[f] = true
	}
	for f := range fb {
		all[f] = true
	}
	names := make([]string, 0, len(all))
	for f := range all {
		names = append(names, f)
	}
	sort.Strings(names)
	var out strings.Builder
	for _, f := range names {
		switch {
		case !fa[f]:
			fmt.Fprintf(&out, "Only in %s: %s\n", b, f)
		case !fb[f]:
			fmt.Fprintf(&out, "Only in %s: %s\n", a, f)
		default:
			da, err := os.ReadFile(filepath.Join(a, f))
			if err != nil {
				return "", err
			}
			db, err := os.ReadFile(filepath.Join(b, f))
			if err != nil {
				return "", err
			}
			if string(da) == string(db) {
				continue
			}
			out.WriteString(UnifiedDiff("a/"+f, "b/"+f, string(da), string(db), 3))
		}
	}
	return out.String(), nil
}

func listFiles(root string) (map[string]bool, error) {
	files := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = true
		return nil
	})
	return files, err
}

type diffOp struct {
	kind byte // ' ', '-', '+'
	text string
}

// lineDiff returns an edit script turning a into b (LCS based, after
// trimming the common prefix and suffix).
func lineDiff(a, b []string) []diffOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	var ops []diffOp
	for _, l := range a[:pre] {
		ops = append(ops, diffOp{' ', l})
	}
	if len(ma)*len(mb) > 50_000_000 {
		// Too large for LCS: show the middle as replaced.
		for _, l := range ma {
			ops = append(ops, diffOp{'-', l})
		}
		for _, l := range mb {
			ops = append(ops, diffOp{'+', l})
		}
	} else {
		n, m := len(ma), len(mb)
		lcs := make([][]int32, n+1)
		for i := range lcs {
			lcs[i] = make([]int32, m+1)
		}
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if ma[i] == mb[j] {
					lcs[i][j] = lcs[i+1][j+1] + 1
				} else if lcs[i+1][j] >= lcs[i][j+1] {
					lcs[i][j] = lcs[i+1][j]
				} else {
					lcs[i][j] = lcs[i][j+1]
				}
			}
		}
		i, j := 0, 0
		for i < n || j < m {
			switch {
			case i < n && j < m && ma[i] == mb[j]:
				ops = append(ops, diffOp{' ', ma[i]})
				i++
				j++
			case j < m && (i == n || lcs[i][j+1] > lcs[i+1][j]):
				ops = append(ops, diffOp{'+', mb[j]})
				j++
			default:
				ops = append(ops, diffOp{'-', ma[i]})
				i++
			}
		}
	}
	for _, l := range a[len(a)-suf:] {
		ops = append(ops, diffOp{' ', l})
	}
	return ops
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// UnifiedDiff renders a unified diff with the given context lines.
func UnifiedDiff(nameA, nameB, a, b string, context int) string {
	ops := lineDiff(splitLines(a), splitLines(b))
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", nameA, nameB)
	// Group changes into hunks.
	i := 0
	lineA, lineB := 1, 1
	type pos struct{ a, b int }
	positions := make([]pos, len(ops))
	for k, op := range ops {
		positions[k] = pos{lineA, lineB}
		switch op.kind {
		case ' ':
			lineA++
			lineB++
		case '-':
			lineA++
		case '+':
			lineB++
		}
	}
	for i < len(ops) {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(i-context, 0)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			// Look ahead: does another change start within 2*context?
			k := end
			for k < len(ops) && ops[k].kind == ' ' && k-end < 2*context {
				k++
			}
			if k < len(ops) && ops[k].kind != ' ' {
				end = k
				continue
			}
			break
		}
		stop := min(end+context, len(ops))
		ca, cb := 0, 0
		for _, op := range ops[start:stop] {
			if op.kind != '+' {
				ca++
			}
			if op.kind != '-' {
				cb++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", positions[start].a, ca, positions[start].b, cb)
		for _, op := range ops[start:stop] {
			out.WriteByte(op.kind)
			out.WriteString(op.text)
			out.WriteByte('\n')
		}
		i = stop
	}
	return out.String()
}
