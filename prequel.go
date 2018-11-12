package main

import (
	"errors"
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

const cursorStatementColor termbox.Attribute = termbox.Attribute(237)

const tempSqlFile string = "prequel.sql"

type Connection struct {
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type Statement struct {
	start  int
	length int
}

var db         *sql.DB
var editor     tui.EditBox
var results    tui.DetailView
var container  tui.Container
var status     tui.Label
var statements []Statement
var statement  Statement

func resizeHandler() {
	editor.Bounds.Width = container.Width
	editor.Bounds.Height = container.Height / 2

	results.Bounds.Top = editor.Bounds.Height
	results.Bounds.Width = container.Width
	results.Bounds.Height = container.Height - editor.Bounds.Height - 1

	status.Bounds.Top = results.Bounds.Bottom() + 1
	status.Bounds.Width = container.Width
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

func cursorInWhichStatement(cur int, ss []Statement) (Statement, error) {
	for _, s := range ss {
		if cur > s.start + s.length - 1 {
			continue
		}

		return s, nil
	}

	// Default to last statement if there is one
	if len(ss) > 0 {
		return ss[len(ss) - 1], nil
	}

	return Statement {}, errors.New("Cursor not in statement")
}

func editorTextChanged(e *tui.EditBox) {
	err := ioutil.WriteFile(tempSqlFile, []byte(e.GetText()), 0644)
	if err != nil {
		panic(err)
	}

	lineHighlighter(e)
}

func lineHighlighter(e *tui.EditBox) {
	var cur, next *tui.Char

	statements = []Statement {}
	statementStart := 0

	chars := e.AllChars()

	for i := 0; i <= len(chars); i++ {
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

		// Statements end at unquoted semi-colons and EOF
		if next == nil ||
		   cur.Quote == tui.QuoteNone && cur.Char == ';' {
			newStatement := Statement {
				start: statementStart,
				length: i - statementStart,
			}

			statementStart = i

			// Statements should include a trailing newline if
			// present.
			if next != nil && next.Char == '\n' {
				newStatement.length++
				statementStart++
			}

			statements = append(statements, newStatement)
		}
	}

	statement, _ = cursorInWhichStatement(e.GetCursor(), statements)

	for i := 0; i < len(chars); i++ {
		if i >= statement.start &&
		   i < statement.start + statement.length {
			chars[i].Bg = cursorStatementColor
		} else {
			chars[i].Bg = termbox.ColorBlack
		}
	}
}

func handleContainerEvent(c *tui.Container, ev escapebox.Event) bool {
	if ev.Type == termbox.EventKey && ev.Key == termbox.KeyF5 {
		runQuery()
		return true
	}

	return false
}

func runQuery() {
	results.Reset()
	status.Text = ""

	query := ""
	for i := statement.start; i < statement.start + statement.length; i++ {
		ch, err := editor.GetChar(i)
		if err != nil {
			panic(err)
		}
		query += string(ch.Char)
	}

	res, err := db.Query(query)
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
			val := "null"
			if values[i] != nil {
				val = fmt.Sprintf("%s", values[i])
			}
			row[i] = val
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

func main() {
	configBytes, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}

	connection := Connection{}
	err = json.Unmarshal(configBytes, &connection)
	if err != nil {
		fmt.Println("Error: config.json, invalid json")
		panic(err)
	}

	if connection.Driver == "" {
		fmt.Println("Error: config.json is missing the 'driver' " +
			    "field");
		return;
	}

	if connection.Database == "" {
		fmt.Println("Error: config.json is missing the 'database' " +
			    "field");
		return;
	}

	db, err = connect(connection)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		panic(err)
	}

	tempSql := "show tables;"
	tempSqlBytes, err := ioutil.ReadFile(tempSqlFile)
	if err == nil {
		tempSql = string(tempSqlBytes);
	}

	tui.Init()
	defer tui.Close()

	editor = tui.EditBox {
		Highlighter:   tui.BasicHighlighter,
		Dialect:       tui.DialectMySQL,
		OnTextChanged: editorTextChanged,
		OnCursorMoved: lineHighlighter,
	}
	editor.SetText(tempSql)

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
		KeyBindingExit: tui.KeyBinding { Key: termbox.KeyCtrlC },
		KeyBindingFocusNext: tui.KeyBinding { Key: termbox.KeyTab },
		KeyBindingFocusPrevious: tui.KeyBinding {
			Seq: tui.SeqShiftTab,
		},
		HandleEvent: handleContainerEvent,
	}

	tui.MainLoop(&container)
}
