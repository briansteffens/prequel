package main

import (
	"fmt"
	"database/sql"
	"github.com/nsf/termbox-go"
	"github.com/briansteffens/escapebox"
	"github.com/briansteffens/tui"
	_ "github.com/go-sql-driver/mysql"
)

var db        *sql.DB
var editor    tui.EditBox
var results   tui.DetailView
var container tui.Container
var status    tui.Label

func resizeHandler() {
	editor.Bounds.Width = container.Width
	editor.Bounds.Height = container.Height / 2

	results.Bounds.Top = editor.Bounds.Height
	results.Bounds.Width = container.Width
	results.Bounds.Height = container.Height - editor.Bounds.Height - 1

	status.Bounds.Top = results.Bounds.Bottom() + 1
	status.Bounds.Width = container.Width
}

func runQuery() {
	results.Columns = []tui.Column{}
	results.Rows = [][]string{}

	tui.Log(editor.GetText())

	res, err := db.Query(editor.GetText())
	if err != nil {
		status.Text = fmt.Sprintf("%s", err)
		return
	}
	defer res.Close()

	columnNames, err := res.Columns()
	if err != nil {
		panic(err)
	}

	values := make([]interface{}, len(columnNames))
	valuePointers := make([]interface{}, len(columnNames))

	for i := 0; i < len(columnNames); i++ {
		valuePointers[i] = &values[i]
	}

	columns := make([]tui.Column, len(columnNames))

	for i := 0; i < len(columnNames); i++ {
		columns[i].Name = columnNames[i]
		columns[i].Width = 15
	}

	rows := make([][]string, 0)

	for res.Next() {
		if err := res.Scan(valuePointers...); err != nil {
			panic(err)
		}

		row := make([]string, len(columnNames))

		for i := 0; i < len(columnNames); i++ {
			row[i] = fmt.Sprintf("%s", values[i])
		}

		rows = append(rows, row)
	}

	results.Columns = columns
	results.Rows = rows
}

func main() {
	tui.Init()
	defer tui.Close()

	var err error

	db, err = sql.Open("mysql", "root@/litgraph")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	editor = tui.EditBox {
		Lines: []string {
			"select * from authors;",
		},
	}

	results = tui.DetailView {
		Columns: []tui.Column {},
		Rows: [][]string {},
		RowBg: termbox.Attribute(0),
		RowBgAlt: termbox.Attribute(236),
		SelectedBg: termbox.Attribute(22),
	}

	status = tui.Label {
	}

	container = tui.Container {
		Controls: []tui.Control {&results, &editor, &status},
		ResizeHandler: resizeHandler,
	}

	// TODO: rework tui.MainLoop() into this?
	c := &container
	c.FocusNext()

	c.Width, c.Height = termbox.Size()
	c.ResizeHandler()

	c.Refresh()

	loop: for {
		ev := escapebox.PollEvent()

		handled := false

		switch ev.Seq {
		case escapebox.SeqNone:
			switch ev.Type {
			case termbox.EventResize:
				c.Width = ev.Width
				c.Height = ev.Height

				if c.ResizeHandler != nil {
					c.ResizeHandler()
				}
			case termbox.EventKey:
				switch ev.Key {
				case termbox.KeyCtrlC:
					break loop
				case termbox.KeyTab:
					c.FocusNext()
					handled = true
				case termbox.KeyF5:
					runQuery()
				}
			}
		case tui.SeqShiftTab:
			c.FocusPrevious()
			handled = true
		}

		if !handled && c.Focused != nil {
			c.Focused.HandleEvent(ev)
		}

		c.Refresh()
	}
}
