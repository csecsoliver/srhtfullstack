package model

import (
	"git.sr.ht/~sircmpwn/core-go/database"
)

type File struct {
	Filename *string `json:"filename"`

	PasteID int
	BlobID  int

	alias  string
	fields *database.ModelFields
}

func (file *File) As(alias string) *File {
	file.alias = alias
	return file
}

func (file *File) Alias() string {
	return file.alias
}

func (file *File) Table() string {
	return "paste_file"
}

func (file *File) Fields() *database.ModelFields {
	if file.fields != nil {
		return file.fields
	}
	file.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "filename", GQL: "filename", Ptr: &file.Filename},

			// Always fetch:
			{SQL: "paste_id", GQL: "", Ptr: &file.PasteID},
			{SQL: "blob_id", GQL: "", Ptr: &file.BlobID},
		},
	}
	return file.fields
}
