prequel
=======

A MySQL query editor with syntax highlighting for the terminal.

![Prequel screenshot](https://s3.amazonaws.com/briansteffens/prequel2.png)

# Downloading and compiling

You'll need git and go installed. Then:

```bash
git clone https://github.com/briansteffens/prequel
cd prequel
make
```

The binary will now be located at `./prequel`.

# Installation

To do a normal install, do this after compiling:

```bash
sudo make install
```

This will install the binary to `/usr/bin/prequel`.

# Uninstallation

Do this to uninstall the binary:

```bash
sudo make uninstall
```

# Connecting to a database

Prequel checks the current directory for a config.json file and uses that to
figure out which database connection settings to use. Copy the example file
and customize it to fit your environment:

```bash
cp config.json.example config.json
vim config.json
```

Once the configuration is done, run the program:

```bash
prequel
```

Prequel is divided into two sections: a query editor on top and a results view
on the bottom. Use the tab key to switch between them.

# Using the query editor

The query editor has vim-inspired shortcuts. There are two modes: command and
insert. Command is the default.

Here are some shortcuts available in command mode:

| Shortcut    | Action                                                        |
|-------------|---------------------------------------------------------------|
| F5          | Run the current query                                         |
| i           | Enter insert mode                                             |
| Tab         | Switch focus to the results view                              |
| h           | Move the cursor left                                          |
| l           | Move the cursor right                                         |
| j           | Move the cursor down                                          |
| k           | Move the cursor up                                            |
| Left arrow  | Move the cursor left                                          |
| Right arrow | Move the cursor right                                         |
| Down arrow  | Move the cursor down                                          |
| Up arrow    | Move the cursor up                                            |
| 0           | Move to the beginning of the current line                     |
| A           | Move to the end of the current line and enter insert mode     |
| o           | Create a new line after the current line and enter insert mode|
| w           | Advance to the next word                                      |
| b           | Move to the previous word                                     |
| x           | Delete the current character                                  |
| gg          | Move to the first character in the first line                 |
| G           | Move to the last character in the last line                   |
| dd          | Delete the current line                                       |
| cw          | Delete the current word and enter insert mode                 |
| Home        | Move to the beginning of the current line                     |
| End         | Move to the end of the current line                           |
| Ctrl+C      | Exit the program                                              |

While in insert mode, you can type normally. The following shortcuts are
available:

| Shortcut    | Action                                                        |
|-------------|---------------------------------------------------------------|
| F5          | Run the current query                                         |
| Escape      | Switch back to command mode                                   |
| Home        | Move to the beginning of the current line                     |
| End         | Move to the end of the current line                           |
| Ctrl+C      | Exit the program                                              |

In the detail view, the following shortcuts are available:

| Shortcut    | Action                                                        |
|-------------|---------------------------------------------------------------|
| Tab         | Switch focus to the query editor                              |
| Home        | Move to the first column in the current row                   |
| End         | Move to the last column in the current row                    |
| Page Up     | Move up one page of rows                                      |
| Page Down   | Move down one page of rows                                    |
| h           | Move the selection to the left one column                     |
| l           | Move the selection to the right one column                    |
| j           | Move the selection down one row                               |
| k           | Move the selection up one row                                 |
| Arrow Keys  | Scroll the viewport without changing the selection            |
| Ctrl+C      | Exit the program                                              |
