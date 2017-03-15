package main

import (
	"errors"
	"fmt"
	"strings"
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

const colorKeyword termbox.Attribute = termbox.ColorBlue
const colorType    termbox.Attribute = termbox.ColorRed

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
var keywords   map[string]termbox.Attribute

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
	results.Reset()

	query := ""
	for i := statement.start; i < statement.start + statement.length; i++ {
		ch, err := editor.GetChar(i)
		if err != nil {
			panic(err)
		}
		query += string(ch.Char)
	}

	tui.Log(query)

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

		ret = append(ret, &tui.Char {
			Char: '\n',
		})
	}

	return ret
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

func logChar(c *tui.Char) string {
	if c == nil {
		return "nil"
	}

	if c.Char == '\n' {
		return "\\n"
	}

	return string(c.Char)
}

func isWhiteSpace(r rune) bool {
	return r != ' ' && r != '\t' && r != '\n'
}

func highlighter(e *tui.EditBox) {
	const quoteNone   rune = 0
	const quoteSingle rune = '\''
	const quoteDouble rune = '"'

	delimiters := []rune { ' ', '\n', '(', ')', ',', ';' }

	var prev, cur, next *tui.Char
	var curEscaped, nextEscaped bool
	var quote rune
	var quoteStartIndex int

	word := ""

	statements = []Statement {}
	statementStart := 0

	chars := charStream(e)

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

		// Handle current character -----------------------------------

		// Check for word delimiter
		isDelimiter := false
		for j := 0; j < len(delimiters); j++ {
			if delimiters[j] == cur.Char {
				isDelimiter = true
				break
			}
		}

		// Reset word if we hit a delimiter or EOF
		if isDelimiter || next == nil {
			wordColor, ok := keywords[word]

			// Color the word if it's a known keyword
			if ok {
				for j := i - 1; j >= i - len(word) - 1; j-- {
					chars[j].Fg = wordColor
				}
			}

			word = ""
		} else {
			word += string(cur.Char)
		}

		// Statements end at unquoted semi-colons and EOF
		if next == nil || quote == quoteNone && cur.Char == ';' {
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

		// Color quotes
		if quote != quoteNone {
			cur.Fg = termbox.ColorGreen
		} else {
			cur.Fg = termbox.ColorWhite
		}

		// Debug logging ----------------------------------------------
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

		wordS := strings.Replace(word, "\n", "\\n", -1)

		tui.Log("%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s %s", logChar(prev),
			logChar(cur), logChar(next),
			quoteS, statementStart, len(statements), wordS,
			curEscapedS, nextEscapedS)

		// Post-handling ----------------------------------------------

		// End quote
		if isCurQuote && quote != quoteNone && !quoteToggledThisLoop &&
		   quote == cur.Char {
			quote = quoteNone
		}

		curEscaped = nextEscaped
	}

	for _, s := range statements {
		tui.Log("statement start=%d length=%d", s.start, s.length)
	}

	statement, _ = cursorInWhichStatement(e.GetCursor(), statements)

	tui.Log("cursor in statement: %d", statement.start)

	for i := 0; i < len(chars); i++ {
		if i >= statement.start &&
		   i < statement.start + statement.length {
			chars[i].Bg = cursorStatementColor
		} else {
			chars[i].Bg = termbox.ColorBlack
		}
	}
}

func main() {
	initKeywords()

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
		OnTextChanged: highlighter,
		OnCursorMoved: highlighter,
	}
	editor.SetText("select * from authors;\nselect * from books;")

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

func initKeywords() {
	keywords = map[string]termbox.Attribute {
		"account":                       colorKeyword,
		"action":                        colorKeyword,
		"add":                           colorKeyword,
		"after":                         colorKeyword,
		"against":                       colorKeyword,
		"aggregate":                     colorKeyword,
		"algorithm":                     colorKeyword,
		"all":                           colorKeyword,
		"alter":                         colorKeyword,
		"always":                        colorKeyword,
		"analyse":                       colorKeyword,
		"analyze":                       colorKeyword,
		"and":                           colorKeyword,
		"any":                           colorKeyword,
		"as":                            colorKeyword,
		"asc":                           colorKeyword,
		"ascii":                         colorKeyword,
		"asensitive":                    colorKeyword,
		"at":                            colorKeyword,
		"autoextend_size":               colorKeyword,
		"auto_increment":                colorKeyword,
		"avg":                           colorKeyword,
		"avg_row_length":                colorKeyword,
		"backup":                        colorKeyword,
		"before":                        colorKeyword,
		"begin":                         colorKeyword,
		"between":                       colorKeyword,
		"bigint":                        colorType,
		"binary":                        colorType,
		"binlog":                        colorKeyword,
		"bit":                           colorType,
		"blob":                          colorType,
		"block":                         colorKeyword,
		"bool":                          colorType,
		"boolean":                       colorType,
		"both":                          colorKeyword,
		"btree":                         colorKeyword,
		"by":                            colorKeyword,
		"byte":                          colorType,
		"cache":                         colorKeyword,
		"call":                          colorKeyword,
		"cascade":                       colorKeyword,
		"cascaded":                      colorKeyword,
		"case":                          colorKeyword,
		"catalog_name":                  colorKeyword,
		"chain":                         colorKeyword,
		"change":                        colorKeyword,
		"changed":                       colorKeyword,
		"channel":                       colorKeyword,
		"char":                          colorType,
		"character":                     colorKeyword,
		"charset":                       colorKeyword,
		"check":                         colorKeyword,
		"checksum":                      colorKeyword,
		"cipher":                        colorKeyword,
		"class_origin":                  colorKeyword,
		"client":                        colorKeyword,
		"close":                         colorKeyword,
		"coalesce":                      colorKeyword,
		"code":                          colorKeyword,
		"collate":                       colorKeyword,
		"collation":                     colorKeyword,
		"column":                        colorKeyword,
		"columns":                       colorKeyword,
		"column_format":                 colorKeyword,
		"column_name":                   colorKeyword,
		"comment":                       colorKeyword,
		"commit":                        colorKeyword,
		"committed":                     colorKeyword,
		"compact":                       colorKeyword,
		"completion":                    colorKeyword,
		"compressed":                    colorKeyword,
		"compression":                   colorKeyword,
		"concurrent":                    colorKeyword,
		"condition":                     colorKeyword,
		"connection":                    colorKeyword,
		"consistent":                    colorKeyword,
		"constraint":                    colorKeyword,
		"constraint_catalog":            colorKeyword,
		"constraint_name":               colorKeyword,
		"constraint_schema":             colorKeyword,
		"contains":                      colorKeyword,
		"context":                       colorKeyword,
		"continue":                      colorKeyword,
		"convert":                       colorKeyword,
		"cpu":                           colorKeyword,
		"create":                        colorKeyword,
		"cross":                         colorKeyword,
		"cube":                          colorKeyword,
		"current":                       colorKeyword,
		"current_date":                  colorKeyword,
		"current_time":                  colorKeyword,
		"current_timestamp":             colorKeyword,
		"current_user":                  colorKeyword,
		"cursor":                        colorKeyword,
		"cursor_name":                   colorKeyword,
		"data":                          colorKeyword,
		"database":                      colorKeyword,
		"databases":                     colorKeyword,
		"datafile":                      colorKeyword,
		"date":                          colorType,
		"datetime":                      colorType,
		"day":                           colorKeyword,
		"day_hour":                      colorKeyword,
		"day_microsecond":               colorKeyword,
		"day_minute":                    colorKeyword,
		"day_second":                    colorKeyword,
		"deallocate":                    colorKeyword,
		"dec":                           colorKeyword,
		"decimal":                       colorType,
		"declare":                       colorKeyword,
		"default":                       colorKeyword,
		"default_auth":                  colorKeyword,
		"definer":                       colorKeyword,
		"delayed":                       colorKeyword,
		"delay_key_write":               colorKeyword,
		"delete":                        colorKeyword,
		"desc":                          colorKeyword,
		"describe":                      colorKeyword,
		"des_key_file":                  colorKeyword,
		"deterministic":                 colorKeyword,
		"diagnostics":                   colorKeyword,
		"directory":                     colorKeyword,
		"disable":                       colorKeyword,
		"discard":                       colorKeyword,
		"disk":                          colorKeyword,
		"distinct":                      colorKeyword,
		"distinctrow":                   colorKeyword,
		"div":                           colorKeyword,
		"do":                            colorKeyword,
		"double":                        colorType,
		"drop":                          colorKeyword,
		"dual":                          colorKeyword,
		"dumpfile":                      colorKeyword,
		"duplicate":                     colorKeyword,
		"dynamic":                       colorKeyword,
		"each":                          colorKeyword,
		"else":                          colorKeyword,
		"elseif":                        colorKeyword,
		"enable":                        colorKeyword,
		"enclosed":                      colorKeyword,
		"encryption":                    colorKeyword,
		"end":                           colorKeyword,
		"ends":                          colorKeyword,
		"engine":                        colorKeyword,
		"engines":                       colorKeyword,
		"enum":                          colorType,
		"error":                         colorKeyword,
		"errors":                        colorKeyword,
		"escape":                        colorKeyword,
		"escaped":                       colorKeyword,
		"event":                         colorKeyword,
		"events":                        colorKeyword,
		"every":                         colorKeyword,
		"exchange":                      colorKeyword,
		"execute":                       colorKeyword,
		"exists":                        colorKeyword,
		"exit":                          colorKeyword,
		"expansion":                     colorKeyword,
		"expire":                        colorKeyword,
		"explain":                       colorKeyword,
		"export":                        colorKeyword,
		"extended":                      colorKeyword,
		"extent_size":                   colorKeyword,
		"false":                         colorKeyword,
		"fast":                          colorKeyword,
		"faults":                        colorKeyword,
		"fetch":                         colorKeyword,
		"fields":                        colorKeyword,
		"file":                          colorKeyword,
		"file_block_size":               colorKeyword,
		"filter":                        colorKeyword,
		"first":                         colorKeyword,
		"fixed":                         colorKeyword,
		"float":                         colorType,
		"float4":                        colorType,
		"float8":                        colorType,
		"flush":                         colorKeyword,
		"follows":                       colorKeyword,
		"for":                           colorKeyword,
		"force":                         colorKeyword,
		"foreign":                       colorKeyword,
		"format":                        colorKeyword,
		"found":                         colorKeyword,
		"from":                          colorKeyword,
		"full":                          colorKeyword,
		"fulltext":                      colorKeyword,
		"function":                      colorKeyword,
		"general":                       colorKeyword,
		"generated":                     colorKeyword,
		"geometry":                      colorKeyword,
		"geometrycollection":            colorKeyword,
		"get":                           colorKeyword,
		"get_format":                    colorKeyword,
		"global":                        colorKeyword,
		"grant":                         colorKeyword,
		"grants":                        colorKeyword,
		"group":                         colorKeyword,
		"group_replication":             colorKeyword,
		"handler":                       colorKeyword,
		"hash":                          colorKeyword,
		"having":                        colorKeyword,
		"help":                          colorKeyword,
		"high_priority":                 colorKeyword,
		"host":                          colorKeyword,
		"hosts":                         colorKeyword,
		"hour":                          colorKeyword,
		"hour_microsecond":              colorKeyword,
		"hour_minute":                   colorKeyword,
		"hour_second":                   colorKeyword,
		"identified":                    colorKeyword,
		"if":                            colorKeyword,
		"ignore":                        colorKeyword,
		"ignore_server_ids":             colorKeyword,
		"import":                        colorKeyword,
		"in":                            colorKeyword,
		"index":                         colorKeyword,
		"indexes":                       colorKeyword,
		"infile":                        colorKeyword,
		"initial_size":                  colorKeyword,
		"inner":                         colorKeyword,
		"inout":                         colorKeyword,
		"insensitive":                   colorKeyword,
		"insert":                        colorKeyword,
		"insert_method":                 colorKeyword,
		"install":                       colorKeyword,
		"instance":                      colorKeyword,
		"int":                           colorType,
		"int1":                          colorType,
		"int2":                          colorType,
		"int3":                          colorType,
		"int4":                          colorType,
		"int8":                          colorType,
		"integer":                       colorType,
		"interval":                      colorKeyword,
		"into":                          colorKeyword,
		"invoker":                       colorKeyword,
		"io":                            colorKeyword,
		"io_after_gtids":                colorKeyword,
		"io_before_gtids":               colorKeyword,
		"io_thread":                     colorKeyword,
		"ipc":                           colorKeyword,
		"is":                            colorKeyword,
		"isolation":                     colorKeyword,
		"issuer":                        colorKeyword,
		"iterate":                       colorKeyword,
		"join":                          colorKeyword,
		"json":                          colorKeyword,
		"key":                           colorKeyword,
		"keys":                          colorKeyword,
		"key_block_size":                colorKeyword,
		"kill":                          colorKeyword,
		"language":                      colorKeyword,
		"last":                          colorKeyword,
		"leading":                       colorKeyword,
		"leave":                         colorKeyword,
		"leaves":                        colorKeyword,
		"left":                          colorKeyword,
		"less":                          colorKeyword,
		"level":                         colorKeyword,
		"like":                          colorKeyword,
		"limit":                         colorKeyword,
		"linear":                        colorKeyword,
		"lines":                         colorKeyword,
		"linestring":                    colorKeyword,
		"list":                          colorKeyword,
		"load":                          colorKeyword,
		"local":                         colorKeyword,
		"localtime":                     colorKeyword,
		"localtimestamp":                colorKeyword,
		"lock":                          colorKeyword,
		"locks":                         colorKeyword,
		"logfile":                       colorKeyword,
		"logs":                          colorKeyword,
		"long":                          colorKeyword,
		"longblob":                      colorType,
		"longtext":                      colorType,
		"loop":                          colorKeyword,
		"low_priority":                  colorKeyword,
		"master":                        colorKeyword,
		"master_auto_position":          colorKeyword,
		"master_bind":                   colorKeyword,
		"master_connect_retry":          colorKeyword,
		"master_delay":                  colorKeyword,
		"master_heartbeat_period":       colorKeyword,
		"master_host":                   colorKeyword,
		"master_log_file":               colorKeyword,
		"master_log_pos":                colorKeyword,
		"master_password":               colorKeyword,
		"master_port":                   colorKeyword,
		"master_retry_count":            colorKeyword,
		"master_server_id":              colorKeyword,
		"master_ssl":                    colorKeyword,
		"master_ssl_ca":                 colorKeyword,
		"master_ssl_capath":             colorKeyword,
		"master_ssl_cert":               colorKeyword,
		"master_ssl_cipher":             colorKeyword,
		"master_ssl_crl":                colorKeyword,
		"master_ssl_crlpath":            colorKeyword,
		"master_ssl_key":                colorKeyword,
		"master_ssl_verify_server_cert": colorKeyword,
		"master_tls_version":            colorKeyword,
		"master_user":                   colorKeyword,
		"match":                         colorKeyword,
		"maxvalue":                      colorKeyword,
		"max_connections_per_hour":      colorKeyword,
		"max_queries_per_hour":          colorKeyword,
		"max_rows":                      colorKeyword,
		"max_size":                      colorKeyword,
		"max_statement_time":            colorKeyword,
		"max_updates_per_hour":          colorKeyword,
		"max_user_connections":          colorKeyword,
		"medium":                        colorType,
		"mediumblob":                    colorType,
		"mediumint":                     colorType,
		"mediumtext":                    colorType,
		"memory":                        colorKeyword,
		"merge":                         colorKeyword,
		"message_text":                  colorKeyword,
		"microsecond":                   colorKeyword,
		"middleint":                     colorKeyword,
		"migrate":                       colorKeyword,
		"minute":                        colorKeyword,
		"minute_microsecond":            colorKeyword,
		"minute_second":                 colorKeyword,
		"min_rows":                      colorKeyword,
		"mod":                           colorKeyword,
		"mode":                          colorKeyword,
		"modifies":                      colorKeyword,
		"modify":                        colorKeyword,
		"month":                         colorKeyword,
		"multilinestring":               colorKeyword,
		"multipoint":                    colorKeyword,
		"multipolygon":                  colorKeyword,
		"mutex":                         colorKeyword,
		"mysql_errno":                   colorKeyword,
		"name":                          colorKeyword,
		"names":                         colorKeyword,
		"national":                      colorKeyword,
		"natural":                       colorKeyword,
		"nchar":                         colorType,
		"ndb":                           colorKeyword,
		"ndbcluster":                    colorKeyword,
		"never":                         colorKeyword,
		"new":                           colorKeyword,
		"next":                          colorKeyword,
		"no":                            colorKeyword,
		"nodegroup":                     colorKeyword,
		"nonblocking":                   colorKeyword,
		"none":                          colorKeyword,
		"not":                           colorKeyword,
		"no_wait":                       colorKeyword,
		"no_write_to_binlog":            colorKeyword,
		"null":                          colorKeyword,
		"number":                        colorType,
		"numeric":                       colorType,
		"nvarchar":                      colorType,
		"offset":                        colorKeyword,
		"old_password":                  colorKeyword,
		"on":                            colorKeyword,
		"one":                           colorKeyword,
		"only":                          colorKeyword,
		"open":                          colorKeyword,
		"optimize":                      colorKeyword,
		"optimizer_costs":               colorKeyword,
		"option":                        colorKeyword,
		"optionally":                    colorKeyword,
		"options":                       colorKeyword,
		"or":                            colorKeyword,
		"order":                         colorKeyword,
		"out":                           colorKeyword,
		"outer":                         colorKeyword,
		"outfile":                       colorKeyword,
		"owner":                         colorKeyword,
		"pack_keys":                     colorKeyword,
		"page":                          colorKeyword,
		"parser":                        colorKeyword,
		"parse_gcol_expr":               colorKeyword,
		"partial":                       colorKeyword,
		"partition":                     colorKeyword,
		"partitioning":                  colorKeyword,
		"partitions":                    colorKeyword,
		"password":                      colorKeyword,
		"phase":                         colorKeyword,
		"plugin":                        colorKeyword,
		"plugins":                       colorKeyword,
		"plugin_dir":                    colorKeyword,
		"point":                         colorKeyword,
		"polygon":                       colorKeyword,
		"port":                          colorKeyword,
		"precedes":                      colorKeyword,
		"precision":                     colorKeyword,
		"prepare":                       colorKeyword,
		"preserve":                      colorKeyword,
		"prev":                          colorKeyword,
		"primary":                       colorKeyword,
		"privileges":                    colorKeyword,
		"procedure":                     colorKeyword,
		"processlist":                   colorKeyword,
		"profile":                       colorKeyword,
		"profiles":                      colorKeyword,
		"proxy":                         colorKeyword,
		"purge":                         colorKeyword,
		"quarter":                       colorKeyword,
		"query":                         colorKeyword,
		"quick":                         colorKeyword,
		"range":                         colorKeyword,
		"read":                          colorKeyword,
		"reads":                         colorKeyword,
		"read_only":                     colorKeyword,
		"read_write":                    colorKeyword,
		"real":                          colorKeyword,
		"rebuild":                       colorKeyword,
		"recover":                       colorKeyword,
		"redofile":                      colorKeyword,
		"redo_buffer_size":              colorKeyword,
		"redundant":                     colorKeyword,
		"references":                    colorKeyword,
		"regexp":                        colorKeyword,
		"relay":                         colorKeyword,
		"relaylog":                      colorKeyword,
		"relay_log_file":                colorKeyword,
		"relay_log_pos":                 colorKeyword,
		"relay_thread":                  colorKeyword,
		"release":                       colorKeyword,
		"reload":                        colorKeyword,
		"remove":                        colorKeyword,
		"rename":                        colorKeyword,
		"reorganize":                    colorKeyword,
		"repair":                        colorKeyword,
		"repeat":                        colorKeyword,
		"repeatable":                    colorKeyword,
		"replace":                       colorKeyword,
		"replicate_do_db":               colorKeyword,
		"replicate_do_table":            colorKeyword,
		"replicate_ignore_db":           colorKeyword,
		"replicate_ignore_table":        colorKeyword,
		"replicate_rewrite_db":          colorKeyword,
		"replicate_wild_do_table":       colorKeyword,
		"replicate_wild_ignore_table":   colorKeyword,
		"replication":                   colorKeyword,
		"require":                       colorKeyword,
		"reset":                         colorKeyword,
		"resignal":                      colorKeyword,
		"restore":                       colorKeyword,
		"restrict":                      colorKeyword,
		"resume":                        colorKeyword,
		"return":                        colorKeyword,
		"returned_sqlstate":             colorKeyword,
		"returns":                       colorKeyword,
		"reverse":                       colorKeyword,
		"revoke":                        colorKeyword,
		"right":                         colorKeyword,
		"rlike":                         colorKeyword,
		"rollback":                      colorKeyword,
		"rollup":                        colorKeyword,
		"rotate":                        colorKeyword,
		"routine":                       colorKeyword,
		"row":                           colorKeyword,
		"rows":                          colorKeyword,
		"row_count":                     colorKeyword,
		"row_format":                    colorKeyword,
		"rtree":                         colorKeyword,
		"savepoint":                     colorKeyword,
		"schedule":                      colorKeyword,
		"schema":                        colorKeyword,
		"schemas":                       colorKeyword,
		"schema_name":                   colorKeyword,
		"second":                        colorKeyword,
		"second_microsecond":            colorKeyword,
		"security":                      colorKeyword,
		"select":                        colorKeyword,
		"sensitive":                     colorKeyword,
		"separator":                     colorKeyword,
		"serial":                        colorKeyword,
		"serializable":                  colorKeyword,
		"server":                        colorKeyword,
		"session":                       colorKeyword,
		"set":                           colorKeyword,
		"share":                         colorKeyword,
		"show":                          colorKeyword,
		"shutdown":                      colorKeyword,
		"signal":                        colorKeyword,
		"signed":                        colorKeyword,
		"simple":                        colorKeyword,
		"slave":                         colorKeyword,
		"slow":                          colorKeyword,
		"smallint":                      colorType,
		"snapshot":                      colorKeyword,
		"socket":                        colorKeyword,
		"some":                          colorKeyword,
		"soname":                        colorKeyword,
		"sounds":                        colorKeyword,
		"source":                        colorKeyword,
		"spatial":                       colorKeyword,
		"specific":                      colorKeyword,
		"sql":                           colorKeyword,
		"sqlexception":                  colorKeyword,
		"sqlstate":                      colorKeyword,
		"sqlwarning":                    colorKeyword,
		"sql_after_gtids":               colorKeyword,
		"sql_after_mts_gaps":            colorKeyword,
		"sql_before_gtids":              colorKeyword,
		"sql_big_result":                colorKeyword,
		"sql_buffer_result":             colorKeyword,
		"sql_cache":                     colorKeyword,
		"sql_calc_found_rows":           colorKeyword,
		"sql_no_cache":                  colorKeyword,
		"sql_small_result":              colorKeyword,
		"sql_thread":                    colorKeyword,
		"sql_tsi_day":                   colorKeyword,
		"sql_tsi_hour":                  colorKeyword,
		"sql_tsi_minute":                colorKeyword,
		"sql_tsi_month":                 colorKeyword,
		"sql_tsi_quarter":               colorKeyword,
		"sql_tsi_second":                colorKeyword,
		"sql_tsi_week":                  colorKeyword,
		"sql_tsi_year":                  colorKeyword,
		"ssl":                           colorKeyword,
		"stacked":                       colorKeyword,
		"start":                         colorKeyword,
		"starting":                      colorKeyword,
		"starts":                        colorKeyword,
		"stats_auto_recalc":             colorKeyword,
		"stats_persistent":              colorKeyword,
		"stats_sample_pages":            colorKeyword,
		"status":                        colorKeyword,
		"stop":                          colorKeyword,
		"storage":                       colorKeyword,
		"stored":                        colorKeyword,
		"straight_join":                 colorKeyword,
		"string":                        colorKeyword,
		"subclass_origin":               colorKeyword,
		"subject":                       colorKeyword,
		"subpartition":                  colorKeyword,
		"subpartitions":                 colorKeyword,
		"super":                         colorKeyword,
		"suspend":                       colorKeyword,
		"swaps":                         colorKeyword,
		"switches":                      colorKeyword,
		"table":                         colorKeyword,
		"tables":                        colorKeyword,
		"tablespace":                    colorKeyword,
		"table_checksum":                colorKeyword,
		"table_name":                    colorKeyword,
		"temporary":                     colorKeyword,
		"temptable":                     colorKeyword,
		"terminated":                    colorKeyword,
		"text":                          colorKeyword,
		"than":                          colorKeyword,
		"then":                          colorKeyword,
		"time":                          colorKeyword,
		"timestamp":                     colorKeyword,
		"timestampadd":                  colorKeyword,
		"timestampdiff":                 colorKeyword,
		"tinyblob":                      colorType,
		"tinyint":                       colorType,
		"tinytext":                      colorType,
		"to":                            colorKeyword,
		"trailing":                      colorKeyword,
		"transaction":                   colorKeyword,
		"trigger":                       colorKeyword,
		"triggers":                      colorKeyword,
		"true":                          colorKeyword,
		"truncate":                      colorKeyword,
		"type":                          colorKeyword,
		"types":                         colorKeyword,
		"uncommitted":                   colorKeyword,
		"undefined":                     colorKeyword,
		"undo":                          colorKeyword,
		"undofile":                      colorKeyword,
		"undo_buffer_size":              colorKeyword,
		"unicode":                       colorKeyword,
		"uninstall":                     colorKeyword,
		"union":                         colorKeyword,
		"unique":                        colorKeyword,
		"unknown":                       colorKeyword,
		"unlock":                        colorKeyword,
		"unsigned":                      colorKeyword,
		"until":                         colorKeyword,
		"update":                        colorKeyword,
		"upgrade":                       colorKeyword,
		"usage":                         colorKeyword,
		"use":                           colorKeyword,
		"user":                          colorKeyword,
		"user_resources":                colorKeyword,
		"use_frm":                       colorKeyword,
		"using":                         colorKeyword,
		"utc_date":                      colorKeyword,
		"utc_time":                      colorKeyword,
		"utc_timestamp":                 colorKeyword,
		"validation":                    colorKeyword,
		"value":                         colorKeyword,
		"values":                        colorKeyword,
		"varbinary":                     colorType,
		"varchar":                       colorType,
		"varcharacter":                  colorKeyword,
		"variables":                     colorKeyword,
		"varying":                       colorKeyword,
		"view":                          colorKeyword,
		"virtual":                       colorKeyword,
		"wait":                          colorKeyword,
		"warnings":                      colorKeyword,
		"week":                          colorKeyword,
		"weight_string":                 colorKeyword,
		"when":                          colorKeyword,
		"where":                         colorKeyword,
		"while":                         colorKeyword,
		"with":                          colorKeyword,
		"without":                       colorKeyword,
		"work":                          colorKeyword,
		"wrapper":                       colorKeyword,
		"write":                         colorKeyword,
		"x509":                          colorKeyword,
		"xa":                            colorKeyword,
		"xid":                           colorKeyword,
		"xml":                           colorKeyword,
		"xor":                           colorKeyword,
		"year":                          colorKeyword,
		"year_month":                    colorKeyword,
		"zerofill":                      colorKeyword,
	}
}
