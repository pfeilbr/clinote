/*
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, see <http://www.gnu.org/licenses/>.
 *
 * Copyright (C) Joakim Kennedy, 2016
 */

package main

import (
	"fmt"

	"github.com/TcM1911/clinote"
	"github.com/spf13/cobra"
)

var newNoteCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new note.",
	Long: `
New creates a new note. A title needs to be given for the
note.

If no notebook is given, the default notebook will be used.

The new note can be open in the $EDITOR by using the edit
flag.`,
	Run: func(cmd *cobra.Command, args []string) {
		title, err := cmd.Flags().GetString("title")
		if err != nil {
			fmt.Println("Error when parsing note title:", err)
			return
		}
		edit, err := cmd.Flags().GetBool("edit")
		if err != nil {
			fmt.Println("Error when parsing edit flag:", err)
			return
		}
		if title == "" && !edit {
			fmt.Println("Note title has to be given")
			return
		}
		notebook, err := cmd.Flags().GetString("notebook")
		if err != nil {
			fmt.Println("Error when parsing notebook name:", err)
			return
		}
		raw, err := cmd.Flags().GetBool("raw")
		if err != nil {
			fmt.Println("Error when parsing raw parameter:", err)
			return
		}
		stdin, err := cmd.Flags().GetBool("stdin")
		if err != nil {
			fmt.Println("Error when parsing stdin parameter:", err)
			return
		}

		createNote(title, notebook, edit, raw, stdin)
	},
}

func init() {
	noteCmd.AddCommand(newNoteCmd)
	newNoteCmd.Flags().StringP("title", "t", "", "Note title.")
	newNoteCmd.Flags().StringP("notebook", "b", "", "The notebook to save note to, if not set the default notebook will be used.")
	newNoteCmd.Flags().BoolP("edit", "e", false, "Open note in the editor.")
	newNoteCmd.Flags().Bool("raw", false, "Edit the content in raw mode.")
	newNoteCmd.Flags().Bool("stdin", false, "Read content from stdin.")
}

func createNote(title, notebook string, edit, raw bool, stdin bool) {
	c := newClient(clinote.DefaultClientOptions)
	defer c.Store.Close()

	note := new(clinote.Note)
	if title == "" {
		note.Title = "Untitled note"
	} else {
		note.Title = title
	}
	if notebook != "" {
		nb, err := clinote.FindNotebook(c.Store, c.NoteStore, notebook)
		if err != nil {
			fmt.Println("Error when searching for notebook:", err)
			return
		}
		note.Notebook = nb
	}
	opts := clinote.DefaultNoteOption
	if raw {
		opts |= clinote.RawNote
	}
	if stdin {
		opts |= clinote.StdinNote
	}

	if edit {
		if err := clinote.CreateAndEditNewNote(c, note, opts); err != nil {
			fmt.Println("Error when editing the note:", err)
		}
		return
	}
	clinote.SaveNewNote(c.NoteStore, note, raw)
}
