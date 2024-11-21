// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// SetQOTDAuthorResult describes the result of setting the quote of the day
// author.
type SetQOTDAuthorResult struct {
	Error *Error `json:"error,omitempty"`
}

// SetQOTDAuthorArgs holds the arguments for setting the quote of the day
// author.
type SetQOTDAuthorArgs struct {
	Entity
	Author string `json:"author"`
}
