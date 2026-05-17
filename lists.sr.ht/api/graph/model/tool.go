package model

import (
	"fmt"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/database"
)

type PatchsetTool struct {
	ID      int       `json:"id"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Details string    `json:"details"`

	PatchsetID int
	RawIcon    string

	alias  string
	fields *database.ModelFields
}

func (tool *PatchsetTool) Icon() ToolIcon {
	icon := ToolIcon(strings.ToUpper(tool.RawIcon))
	if !icon.IsValid() {
		panic(fmt.Errorf("patchset_tool %d has invalid icon %s",
			tool.ID, tool.RawIcon))
	}
	return icon
}

func (tool *PatchsetTool) As(alias string) *PatchsetTool {
	tool.alias = alias
	return tool
}

func (tool *PatchsetTool) Alias() string {
	return tool.alias
}

func (tool *PatchsetTool) Table() string {
	return "patchset_tool"
}

func (tool *PatchsetTool) Fields() *database.ModelFields {
	if tool.fields != nil {
		return tool.fields
	}
	tool.fields = &database.ModelFields{
		Fields: []*database.FieldMap{
			{SQL: "updated", GQL: "updated", Ptr: &tool.Updated},
			{SQL: "icon", GQL: "icon", Ptr: &tool.RawIcon},
			{SQL: "details", GQL: "details", Ptr: &tool.Details},

			// Always fetch:
			{SQL: "id", GQL: "", Ptr: &tool.ID},
			{SQL: "created", GQL: "", Ptr: &tool.Created},
			{SQL: "patchset_id", GQL: "", Ptr: &tool.PatchsetID},
		},
	}
	return tool.fields
}
