package main

import (
	"github.com/nsf/termbox-go"
	"github.com/briansteffens/tui"
)

var editor    tui.EditBox
var results   tui.DetailView
var container tui.Container

func resizeHandler() {
	editor.Bounds.Width = container.Width
	editor.Bounds.Height = container.Height / 2

	results.Bounds.Top = editor.Bounds.Height
	results.Bounds.Width = container.Width
	results.Bounds.Height = container.Height - editor.Bounds.Height
}

func main() {
	tui.Init()
	defer tui.Close()

	editor = tui.EditBox {
		Lines: []string {
			"select * from users;",
		},
	}

	results = tui.DetailView {
		Columns: []tui.Column {
			tui.Column { Name: "ID", Width: 5 },
			tui.Column { Name: "Name", Width: 15 },
			tui.Column { Name: "Email", Width: 25 },
		},
		Rows: [][]string {
			[]string { "3", "Brian", "brian@brian.com" },
			[]string { "7", "Other Brian", "other@brian.com" },
			[]string { "13", "Another Brian",
				   "another@brian.com" },
			[]string { "17", "More Brians", "more@brian.com" },
		},
		RowBg: termbox.Attribute(0),
		RowBgAlt: termbox.Attribute(236),
		SelectedBg: termbox.Attribute(22),
	}

	container = tui.Container {
		Controls: []tui.Control {&results, &editor},
		ResizeHandler: resizeHandler,
	}

	tui.MainLoop(&container)
}
