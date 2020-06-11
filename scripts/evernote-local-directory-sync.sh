#!/bin/bash

# output: "2020-02-12 10:10:56"
# date -r /Users/pfeilbr/tmp/a.txt "+%Y-%m-%d %H:%M:%S"
SCRIPT_NAME=$(basename "$0")
SCRIPT_BARE_NAME=$(echo $SCRIPT_NAME | cut -d"." -f1)

CONFIG_DIRECTORY=~/.$SCRIPT_BARE_NAME
[ -d "${CONFIG_DIRECTORY}" ] || mkdir -p "${CONFIG_DIRECTORY}"

LOG_DIRECTORY="${CONFIG_DIRECTORY}/logs"
[ -d "${LOG_DIRECTORY}" ] || mkdir -p "${LOG_DIRECTORY}"

CONTENT_HASH_DIRECTORY="${CONFIG_DIRECTORY}/content-hashes"
[ -d "${CONTENT_HASH_DIRECTORY}" ] || mkdir -p "${CONTENT_HASH_DIRECTORY}"

VALIDATION_ERRORS_DIRECTORY="${CONFIG_DIRECTORY}/validation-errors"
[ -d "${VALIDATION_ERRORS_DIRECTORY}" ] || mkdir -p "${VALIDATION_ERRORS_DIRECTORY}"


SOURCE_DIRECTORY=~/Dropbox/notes
DEFAULT_NOTEBOOK_NAME="Inbox"
#SOURCE_FILES="${SOURCE_DIRECTORY}/2014-05*.md"
SOURCE_FILES="${SOURCE_DIRECTORY}/*.md"
CLI_PATH=clinote

function contains_string {
    source_string=$1
    search_string=$2

    if grep -q "$search_string" <<< "$source_string"; then
        return 0
    else
        return 1
    fi
}

function note_exists_on_server {
    note_title=$1
    clinote note "${note_title}" > /dev/null 2>&1
    exit_status=$?
    if [ $exit_status -eq 0 ]; then
        echo "true"
    else
        echo "false"
    fi
}

function file_contents_changed {
    file_path=$1
    file_name=$(basename "$file_path")    
    last_hash_file_path="${CONTENT_HASH_DIRECTORY}/${file_name}.md5"
    ret=0
    if [ -f "${last_hash_file_path}" ]; then
        last_hash=$(cat "${last_hash_file_path}")
        current_hash=$(md5 -q "${file_path}")

        if [ "${last_hash}" = "${current_hash}" ]; then
            ret=1
        fi
    fi
    return $ret
}

function get_validation_error_file_flag_path {
    file_path=$1
    file_name=$(basename "$file_path")
    echo "${VALIDATION_ERRORS_DIRECTORY}/${file_name}"
}

function set_validation_error_file_flag {
    file_path=$1
    validation_error_file_flag_path=$(get_validation_error_file_flag_path "${file_path}")
    cp "${file_path}" "${validation_error_file_flag_path}"
}

function has_validation_errors {
    file_path=$1
    file_name=$(basename "$file_path")
    ret=1

    validation_error_file_flag_path=$(get_validation_error_file_flag_path "${file_path}")

    if [ -f "${validation_error_file_flag_path}" ]; then
        ret=0
    fi

    return $ret
}

function should_process_file {
    file_path="$1"
    file_name=$(basename "$file_path")    

    if has_validation_errors "${file_path}"; then
        return 1
    fi

    if file_contents_changed "${file_path}"; then
        return 0
    fi
    
    return 1
}

function create_local_file_contents_hash {
    file_path=$1
    file_name=$(basename "$file_path")    
    last_hash_file_path="${CONTENT_HASH_DIRECTORY}/${file_name}.md5"
    md5 -q "${file_path}" > "${last_hash_file_path}"
}

function create_or_update_note_from_file {
    file_path=$1
    file_name=$(basename "$file_path")    

    if ! should_process_file "${file_path}"; then
        echo "not processing: ${file_path}"
        return 0
    fi

    # delete note if already exists on server
    exists_on_server=$(note_exists_on_server "${file_name}")
    if [ "${exists_on_server}" == "true" ]; then
        delete_note_cmd="${CLI_PATH} note delete \"${file_name}\""
        echo "${delete_note_cmd}"
        eval "${delete_note_cmd}"
    fi

    new_note_cmd="cat \"${file_path}\" | ${CLI_PATH} note new --title \"${file_name}\" --stdin --edit"
    echo "creating note from \""${file_path}\"""
    output=$(eval "${new_note_cmd}")
    if contains_string "${output}" "EDAMUserException"; then
        echo "failed to create new note for file: ${file_path}"
        echo "${output}"

        if contains_string "${output}" "ENML_VALIDATION"; then
            echo "XML Validation Error"
            set_validation_error_file_flag "${file_path}"
        fi
    else
        # success - create local file contents hash
        create_local_file_contents_hash "${file_path}"
    fi
    sleep 3
}

for file_path in $SOURCE_FILES
do
    create_or_update_note_from_file "${file_path}"
    echo -e "\n"
done


# exit

# property defaultNotebookName : "${DEFAULT_NOTEBOOK_NAME}"
# tell application "Evernote"
#     --create note title "${NOTE_TITLE}" with text "Here is my new text note" notebook defaultNotebookName
#     create note title "${NOTE_TITLE}" from file "${file_path}" notebook defaultNotebookName
#     log "${NOTE_TITLE}"
#     log "${file_path}"
# end tell

# osascript -l JavaScript<<EOD
# var l = console.log;
# //var App = new Application("/Applications/Evernote.app");
# var App = new Application("com.evernote.Evernote");

# var defaultNotebookName="${DEFAULT_NOTEBOOK_NAME}";

# //App.activate();


# //var notes = App.findNotes("intitle:aws-static-site-patterns-email");
# //l(JSON.stringify(notes[0].title()));

# //l("${file_path}")
# //var note = App.findNote("intitle:aws-static-site-patterns-email");
# //l(note.title());

# var title = "${NOTE_TITLE}";
# var filePath = "${file_path}";

# l(defaultNotebookName)
# l(title)
# l(filePath)
# var note = App.createNote({notebook: defaultNotebookName, title: title, fromFile: filePath})
# l(note.title());
# EOD