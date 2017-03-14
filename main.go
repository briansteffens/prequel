package main

import (
	"fmt"
	"io/ioutil"
	"encoding/json"
	"database/sql"
	"github.com/nsf/termbox-go"
	"github.com/briansteffens/escapebox"
	"github.com/briansteffens/tui"
	_ "github.com/go-sql-driver/mysql"
)

const minColumnWidth int = 5
const maxColumnWidth int = 25

type Connection struct {
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

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

	columns := make([]tui.Column, len(columnNames))

	for i := 0; i < len(columnNames); i++ {
		columns[i].Name = columnNames[i]

		width := len(columns[i].Name)

		for _, row := range rows {
			if len(row[i]) > width {
				width = len(row[i])
			}
		}

		width++

		if width < minColumnWidth {
			width = minColumnWidth
		}

		if width > maxColumnWidth {
			width = maxColumnWidth
		}

		columns[i].Width = width
	}

	results.Columns = columns
	results.Rows = rows
}

func connect(conn Connection) (*sql.DB, error) {
	dsn := conn.User

	if conn.Password != "" {
		dsn += ":" + conn.Password
	}

	if dsn != "" {
		dsn += "@"
	}

	dsn += fmt.Sprintf("tcp(%s:%d)", conn.Host, conn.Port)

	if conn.Database != "" {
		dsn += "/" + conn.Database
	}

	return sql.Open(conn.Driver, dsn)
}

func charStream(e *tui.EditBox) []*tui.Char {
	ret := []*tui.Char {}

	for l := 0; l < len(e.Lines); l++ {
		line := &e.Lines[l]

		for c := 0; c < len(*line); c++ {
			ret = append(ret, &(*line)[c])
		}
	}

	return ret
}

func colorizer(e *tui.EditBox) {
	chars := charStream(e)

	var prev, cur, next *tui.Char
	var curEscaped, nextEscaped bool
	var quote rune
	var quoteStartIndex int

	const quoteNone   rune = 0
	const quoteSingle rune = '\''
	const quoteDouble rune = '"'

	// Loop over all chars plus one. i is always the index of 'next' so
	// the loop is basically running one char ahead. Run one extra time
	// to process the last character, which at that point will be in cur.
	for i := 0; i <= len(chars); i++ {
		prev = cur
		cur = next

		if i < len(chars) {
			next = chars[i]
		} else {
			next = nil
		}

		// Skip first iteration because cur won't be set yet.
		if cur == nil {
			continue
		}

		// Is the next character:
		//   - Preceded by a slash
		nextSlashEscaped := next != nil && cur.Char == '\\'

		// Is the next character:
		//   - A quote char of the same type as the quote it's inside
		//   - Preceded by another of the same quote char type
		//   - Not the second character in a quote
		nextDoubleEscaped := next != nil && next.Char == quote &&
				     cur.Char == quote && quoteStartIndex < i

		// Is the next character:
		//   - Either slash- or double-escaped
		//   - Not preceded by another escaped character
		nextEscaped = !curEscaped &&
			      (nextSlashEscaped || nextDoubleEscaped)

		// Is the current character:
		//   - A quote char
		//   - Not escaped
		//   - Not the first in a double-escaped sequence ('' or "")
		isCurQuote := !curEscaped && !nextDoubleEscaped &&
			      (cur.Char == quoteSingle ||
			       cur.Char == quoteDouble)

		quoteToggledThisLoop := false

		// Start of a quote
		if isCurQuote && quote == quoteNone {
			quote = cur.Char
			quoteToggledThisLoop = true
			quoteStartIndex = i
		}

		// Handling of cur
		if quote != quoteNone {
			cur.Fg = termbox.ColorGreen
		} else {
			cur.Fg = termbox.ColorWhite
		}

		// Debug logging
		prevS := "nil"
		if prev != nil {
			prevS = string(prev.Char)
		}

		curS := "nil"
		if cur != nil {
			curS = string(cur.Char)
		}

		nextS := "nil"
		if next != nil {
			nextS = string(next.Char)
		}

		quoteS := "nil"
		if quote != quoteNone {
			quoteS = string(quote)
		}

		curEscapedS := ""
		if curEscaped {
			curEscapedS = "curEscaped"
		}

		nextEscapedS := ""
		if nextEscaped {
			nextEscapedS = "nextEscaped"
		}

		tui.Log("%s\t%s\t%s\t%s\t%s %s", prevS, curS, nextS,
			quoteS, curEscapedS, nextEscapedS)

		// Post-handling

		// End quote
		if isCurQuote && quote != quoteNone && !quoteToggledThisLoop &&
		   quote == cur.Char {
			quote = quoteNone
		}

		curEscaped = nextEscaped
	}
}

func main() {
	tui.Init()
	defer tui.Close()

	configBytes, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}

	connection := Connection{}
	json.Unmarshal(configBytes, &connection)

	db, err = connect(connection)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	editor = tui.EditBox {
		OnTextChanged: colorizer,
	}
	editor.SetText("a'b\\'c''d'e")

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
