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
 * Copyright (C) Joakim Kennedy, 2016-2018
 */

package clinote

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/TcM1911/clinote/markdown"
	uuid "github.com/satori/go.uuid"
)

const (
	// XMLHeader is the header that needs to added to the note content.
	XMLHeader = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE en-note SYSTEM "http://xml.evernote.com/pub/enml2.dtd">`
	// headSep indicates the start and end of the note header
	headSpace             = " "
	headSep               = "---"
	headTitleField        = "title:"
	headNotebookNameField = "notebook:"
	newNotePrependString  = "new_note_"
)

var (
	// NoteFilterOrderCreated sorts the notes by create time.
	NoteFilterOrderCreated = int32(1)
	// NoteFilterOrderUpdated sorts the notes by update time.
	NoteFilterOrderUpdated = int32(2)
	// NoteFilterOrderRelevance sorts the notes by relevance.
	NoteFilterOrderRelevance = int32(3)
	// NoteFilterOrderSequenceNumber sorts the notes by sequence number.
	NoteFilterOrderSequenceNumber = int32(4)
	// NoteFilterOrderTitle sorts the notes by title.
	NoteFilterOrderTitle = int32(5)
)

var (
	// ErrNoNoteFound is returned if search resulted in no notes found.
	ErrNoNoteFound = errors.New("no note found")
)

// NoteOption are used for options around notes.
type NoteOption int32

const (
	// DefaultNoteOption will display or edit the note with default options.
	DefaultNoteOption NoteOption = 0
	// RawNote option will display or edit the note in it's raw format.
	RawNote = 1 << iota
	// UseRecoveryPointNote should be used to signal that the user wants to
	// reopen the note that the note store failed to save.
	UseRecoveryPointNote
	// StdinNote will read note contents from stdin
	StdinNote
)

// Note is the structure of an Evernote note.
type Note struct {
	// Title is the note tile.
	Title string
	// GUID is the unique identifier.
	GUID string
	// Body contains the body of the note.
	Body string `xml:",innerxml"`
	// MD is a Markdown representation of the note body.
	MD string
	// Deleted is set true if the note is marked for deletion.
	Deleted bool
	// Notebook the note belongs to.
	Notebook *Notebook
	// Created
	Created int64
	// Updated
	Updated int64
}

// Hash returns the hash for the note. If raw equals true, the raw
// content is used in the hash. Otherwise, the MD content is used.
func (n *Note) Hash(raw bool) []byte {
	hasher := md5.New()
	hasher.Write([]byte(n.Title))
	if raw {
		hasher.Write([]byte(n.Body))
	} else {
		hasher.Write([]byte(n.MD))
	}
	return hasher.Sum(nil)
}

// NoteFilter is the search filter for notes.
type NoteFilter struct {
	// NotebookGUID is the GUID for the notebook to limit the search to.
	NotebookGUID string
	// Words can be a search string or note title.
	Words string
	// Order
	Order int32
}

// FindNotes searches for notes.
func FindNotes(ns NotestoreClient, filter *NoteFilter, offset int, count int) ([]*Note, error) {
	return ns.FindNotes(filter, offset, count)
}

// GetNote gets the note metadata in the notebook from the server.
// If the notebook is an empty string, the first matching note will
// be returned.
func GetNote(db Storager, ns NotestoreClient, title, notebook string) (*Note, error) {
	// Check if the title is a number. If it is
	// assume that the user wants to get the note
	// from a saved search.
	index, err := strconv.Atoi(title)
	if err == nil && index > 0 {
		// Get note from saved search
		notes, err := db.GetSearch()
		if err != nil {
			return nil, err
		}
		if index <= len(notes) {
			return notes[index-1], nil
		}
	}

	filter := new(NoteFilter)
	if notebook != "" {
		nb, err := findNotebook(db, ns, notebook)
		if err != nil {
			return nil, err
		}
		filter.NotebookGUID = nb.GUID
	}
	filter.Words = title
	notes, err := ns.FindNotes(filter, 0, 20)
	if err != nil {
		return nil, err
	}
	var note *Note
	for _, n := range notes {
		if n.Title == title {
			note = n
			break
		}
	}
	if note == nil {
		return nil, ErrNoNoteFound
	}
	return note, nil
}

// GetNoteWithContent returns the note with content from the user's notestore.
func GetNoteWithContent(db Storager, ns NotestoreClient, title string) (*Note, error) {
	n, err := GetNote(db, ns, title, "")
	content, err := ns.GetNoteContent(n.GUID)
	if err != nil {
		return nil, err
	}
	err = decodeXML(content, n)
	if err != nil {
		return nil, err
	}
	n.MD, err = markdown.FromHTML(n.Body)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// SaveChanges updates the changes to the note on the server.
func SaveChanges(ns NotestoreClient, n *Note, opts NoteOption) error {
	return saveChanges(ns, n, true, opts&RawNote != 0)
}

// ChangeTitle changes the note's title.
func ChangeTitle(db Storager, ns NotestoreClient, old, new string) error {
	n, err := GetNote(db, ns, old, "")
	if err != nil {
		return err
	}
	n.Title = new
	return saveChanges(ns, n, false, false)
}

// MoveNote moves the note to a new notebook.
func MoveNote(db Storager, ns NotestoreClient, noteTitle, notebookName string) error {
	n, err := GetNote(db, ns, noteTitle, "")
	if err != nil {
		return err
	}
	b, err := FindNotebook(db, ns, notebookName)
	if err != nil {
		return err
	}
	n.Notebook = b
	return saveChanges(ns, n, false, false)
}

// DeleteNote moves a note from the notebook to the trash can.
func DeleteNote(db Storager, ns NotestoreClient, title, notebook string) error {
	n, err := GetNote(db, ns, title, notebook)
	if err != nil {
		return err
	}
	err = ns.DeleteNote(n.GUID)
	if err != nil {
		return err
	}
	return nil
}

func saveChanges(ns NotestoreClient, n *Note, updateContent, useRawContent bool) error {
	if updateContent {
		body := toXML(n.MD)
		if useRawContent {
			body = fmt.Sprintf("%s<en-note>%s</en-note>", XMLHeader, n.Body)
		}
		n.Body = body
	}
	err := ns.UpdateNote(n)
	if err != nil {
		return err
	}
	return nil
}

// SaveNewNote pushes the new note to the server.
func SaveNewNote(ns NotestoreClient, n *Note, raw bool) error {
	var body string
	if !raw && n.MD != "" {
		body = toXML(n.MD)
	} else if raw {
		body = fmt.Sprintf("%s<en-note>%s</en-note>", XMLHeader, n.Body)
	} else {
		body = XMLHeader + "<en-note></en-note>"
	}
	n.Body = body
	if err := ns.CreateNote(n); err != nil {
		return err
	}
	return nil
}

// EditNote opens the editor so the user can edit the note. Once the user closes the
// editor, the note is saved to the notestore.
func EditNote(client *Client, title string, opts NoteOption) error {
	db, ns := client.Store, client.NoteStore
	var note *Note
	var err error
	if opts&UseRecoveryPointNote != 0 {
		note, err = db.GetNoteRecoveryPoint()
		if note.GUID == "" {
			return ErrNoNoteFound
		}
	} else {
		note, err = GetNoteWithContent(db, ns, title)
	}
	if err != nil {
		return err
	}
	oldHash := note.Hash(opts&RawNote != 0)
	nb, err := GetNotebook(client.NoteStore, note.Notebook.GUID)
	if err != nil {
		return err
	}
	note.Notebook = nb
	initialNotebook := getNotebookName(note)
	cacheFile, err := editNote(client, note, opts)
	if err != nil {
		return err
	}
	defer cacheFile.CloseAndRemove()
	err = parseNote(cacheFile, note, opts)
	if err != nil {
		return err
	}
	err = checkForNotebookAndUpdate(client, note, initialNotebook)
	if err != nil {
		return err
	}
	if bytes.Equal(oldHash, note.Hash(opts&RawNote != 0)) && initialNotebook == note.Notebook.Name {
		return nil
	}
	err = SaveChanges(ns, note, opts)
	if err != nil {
		saveErr := db.SaveNoteRecoveryPoint(note)
		if saveErr != nil {
			err = errors.New("Error when saving note: " + err.Error() + "\nFailed to create recovery point: " + saveErr.Error())
		}
	}
	return err
}

// CreateAndEditNewNote creates a new note and opens it in the client's editor.
// Once the editor has been closed, the note is saved to the notestore.
func CreateAndEditNewNote(client *Client, note *Note, opts NoteOption) error {
	initialNotebook := getNotebookName(note)
	cacheFile, err := editNote(client, note, opts)
	if err != nil {
		return err
	}

	defer cacheFile.CloseAndRemove()
	err = parseNote(cacheFile, note, opts)
	if err != nil {
		return err
	}
	err = checkForNotebookAndUpdate(client, note, initialNotebook)
	if err != nil {
		return err
	}
	return SaveNewNote(client.NoteStore, note, opts&RawNote != 0)
}

func checkForNotebookAndUpdate(client *Client, note *Note, initialNotebook string) error {
	if note.Notebook == nil || initialNotebook == note.Notebook.Name {
		return nil
	}
	b, err := FindNotebook(client.Store, client.NoteStore, note.Notebook.Name)
	if err != nil {
		return err
	}
	note.Notebook = b
	return nil
}

// getNotebookName returns the Notebook name or an empty string.
func getNotebookName(note *Note) string {
	if note.Notebook == nil {
		return ""
	}
	return note.Notebook.Name
}

func randomFilename(prepend string) (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return prepend + id.String(), nil
}

func editNote(client *Client, note *Note, opts NoteOption) (CacheFile, error) {
	filename := ""

	// If the note has a GUID == "", it is a new note.
	// In this case, generate a random filename.
	if note.GUID == "" {
		randName, err := randomFilename(newNotePrependString)
		if err != nil {
			return nil, err
		}
		filename += randName
	}

	if opts&RawNote != 0 {
		// Since the GUID is an empty string for new notes, we can allow a append of it.
		filename += note.GUID + ".xml"
	} else {
		// body = note.MD
		filename += note.GUID + ".md"
	}
	cacheFile, err := client.NewCacheFile(filename)
	if err != nil {
		return nil, err
	}

	if opts&StdinNote != 0 {
		bytes, _ := ioutil.ReadAll(os.Stdin)
		note.MD = string(bytes)
		note.Body = string(bytes)
	}

	err = WriteNote(cacheFile, note, opts)
	if err != nil {
		return nil, err
	}
	// XXX: We need to close the file handler to the file
	// before it is handed over to the editor. Otherwise,
	// Go doesn't detect the changes.
	err = cacheFile.Close()
	if err != nil {
		return nil, err
	}

	if opts&StdinNote == 0 {
		err = client.Edit(cacheFile)
		if err != nil {
			return nil, err
		}
	}
	err = cacheFile.ReOpen()
	if err != nil {
		return nil, err
	}
	return cacheFile, nil
}

func parseNote(r io.Reader, n *Note, opts NoteOption) error {
	scanner := bufio.NewScanner(r)
	if err := parseHeader(scanner, n); err != nil {
		return err
	}
	return parseContent(scanner, n, opts)
}

func parseHeader(scanner *bufio.Scanner, n *Note) error {
	// Find beginning of the header.
	for scanner.Scan() {
		if scanner.Text() == headSep {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Parse header until the end.
	for scanner.Scan() {
		line := scanner.Text()
		// End of header
		if line == headSep {
			break
		}

		if strings.Index(line, headTitleField) == 0 {
			n.Title = strings.TrimSpace(line[len(headTitleField):])
			continue
		}

		if strings.Index(line, headNotebookNameField) == 0 {
			if n.Notebook == nil {
				n.Notebook = new(Notebook)
			}
			n.Notebook.Name = strings.TrimSpace(line[len(headNotebookNameField):])
		}
	}
	return scanner.Err()
}

func parseContent(scanner *bufio.Scanner, n *Note, opts NoteOption) error {
	buf := new(bytes.Buffer)
	for scanner.Scan() {
		buf.WriteString(scanner.Text() + "\n")
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if opts&RawNote != 0 {
		n.Body = strings.Trim(buf.String(), "\n")
	} else {
		n.MD = strings.Trim(buf.String(), "\n")
	}
	return nil
}

func writeNoteHeader(w io.Writer, n *Note) error {
	a := []string{
		headSep,
		headTitleField + " " + n.Title,
	}
	if n.Notebook != nil && n.Notebook.Name != "" {
		a = append(a, headNotebookNameField+headSpace+n.Notebook.Name)
	}
	a = append(a, headSep)
	for _, line := range a {
		_, err := w.Write([]byte(line + "\n"))
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteNote writes the note using the provided writer.
func WriteNote(w io.Writer, n *Note, opts NoteOption) error {
	if err := writeNoteHeader(w, n); err != nil {
		return err
	}
	var err error
	if opts&RawNote != 0 {
		_, err = w.Write([]byte(n.Body))
	} else {
		_, err = w.Write([]byte(n.MD))
	}
	if err != nil {
		return err
	}
	// Add an extra line to ensure note ends with a new line.
	_, err = w.Write([]byte("\n"))
	if err != nil {
		return err
	}
	return err
}

func toXML(mdBody string) string {
	b := []byte("")
	content := bytes.NewBuffer(b)
	content.WriteString(XMLHeader)
	content.WriteString("<en-note>")
	content.Write(markdown.ToXML(mdBody))
	content.WriteString("</en-note>")
	return content.String()
}

func decodeXML(content string, v interface{}) error {
	d := xml.NewDecoder(strings.NewReader(content))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.AutoClose = xml.HTMLAutoClose
	return d.Decode(&v)
}
