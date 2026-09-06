// Command diskette is the phase-0 skeleton of the dual-pane file manager: a
// single navigable local directory pane built on the graphite TUI
// framework. Later phases add the second pane, remote (SFTP) panes, and
// the copy engine.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/pathutil"
)

// row is one line of the directory listing: either a real entry or the
// synthetic ".." row used to navigate to the parent directory.
type row struct {
	name     string
	isDir    bool
	isParent bool
}

func main() {
	start, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "diskette:", err)
		os.Exit(1)
	}

	app := Graphite.NewApplication()
	win := Graphite.NewWindow(0, 0, "")
	win.SetPercentSize(90, 90)

	var (
		list *Graphite.ListBox
		dir  string
		rows []row
	)
	var load func(target string)

	onSelect := func(idx int, _ string) {
		if idx < 0 || idx >= len(rows) {
			return
		}
		switch r := rows[idx]; {
		case r.isParent:
			if parent, ok := pathutil.Parent(dir); ok {
				load(parent)
			}
		case r.isDir:
			load(filepath.Join(dir, r.name))
		}
	}

	load = func(target string) {
		entries, err := os.ReadDir(target)
		if err != nil {
			return
		}

		dir = filepath.Clean(target)
		win.Title = " " + dir + " "

		var dirs, files []os.DirEntry
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e)
			} else {
				files = append(files, e)
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
		sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

		rows = rows[:0]
		items := make([]string, 0, len(dirs)+len(files)+1)
		if _, ok := pathutil.Parent(dir); ok {
			rows = append(rows, row{isParent: true})
			items = append(items, "  ..")
		}
		for _, e := range dirs {
			rows = append(rows, row{name: e.Name(), isDir: true})
			items = append(items, "  /"+e.Name())
		}
		for _, e := range files {
			rows = append(rows, row{name: e.Name()})
			items = append(items, "  "+e.Name())
		}

		if list == nil {
			list = Graphite.NewListBox(0, 2, -1, -2, items, onSelect)
			win.AddWidget(list)
		} else {
			list.Items = items
			list.Selected = 0
			list.Scroll = 0
		}
	}

	load(start)

	app.SetWindow(win)
	app.SetStatus("Enter: open   Esc: quit")
	app.Run()
}
