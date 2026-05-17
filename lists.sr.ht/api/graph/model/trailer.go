package model

import (
	"fmt"
	"strings"
)

type Trailer struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func ParseTrailer(in string) *Trailer {
	name, value, found := strings.Cut(in, ": ")
	if !found {
		return nil
	}
	name = strings.Trim(name, " \t")
	value = strings.Trim(value, " \t")
	if len(name) == 0 || len(value) == 0 {
		return nil
	}
	return &Trailer{
		Name:  name,
		Value: value,
	}
}

func (t Trailer) String() string {
	return fmt.Sprintf("%s: %s", t.Name, t.Value)
}
