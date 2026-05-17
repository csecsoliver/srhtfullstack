package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

// XXX: gqlgen bug prevents us from using type URL *url.URL
type URL struct {
	*url.URL
}

func (u *URL) UnmarshalGQL(v any) error {
	raw, ok := v.(string)
	if !ok {
		return fmt.Errorf("mail format is a base64-encoded string")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	u.URL = parsed
	return nil
}

func (u URL) MarshalGQL(w io.Writer) {
	data, err := json.Marshal(u.String())
	if err != nil {
		panic(err)
	}
	w.Write(data)
}
