package server

import (
	"encoding/json"
	"testing"
)

// TestAnExplicitlyEmptyDisabledToolsListIsNotAnAbsentOne is the wire half of
// the reversible-disable fix. The handler used to gate on
// `len(req.DisabledTools) > 0`, so the one request that means "disable nothing"
// was indistinguishable from a request that never mentioned tools — and a
// session that had disabled a tool could not be told to give it back.
//
// The distinction survives decoding, so the handler can gate on nil instead.
func TestAnExplicitlyEmptyDisabledToolsListIsNotAnAbsentOne(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantNil  bool
		wantLen  int
		explains string
	}{
		{
			name:     "field absent",
			body:     `{"model":"claude"}`,
			wantNil:  true,
			explains: "a request that says nothing about tools must leave the tool set alone",
		},
		{
			name:     "field explicitly empty",
			body:     `{"disabled_tools":[]}`,
			wantNil:  false,
			wantLen:  0,
			explains: "an empty list is how a caller re-enables every tool",
		},
		{
			name:     "field explicitly null",
			body:     `{"disabled_tools":null}`,
			wantNil:  true,
			explains: "null carries no set, so it reads the same as absent",
		},
		{
			name:    "field populated",
			body:    `{"disabled_tools":["shell"]}`,
			wantNil: false,
			wantLen: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req ConfigRequest
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("decode %s: %v", tc.body, err)
			}
			if gotNil := req.DisabledTools == nil; gotNil != tc.wantNil {
				t.Fatalf("%s decoded to nil=%v, want nil=%v — %s", tc.body, gotNil, tc.wantNil, tc.explains)
			}
			if !tc.wantNil && len(req.DisabledTools) != tc.wantLen {
				t.Errorf("%s decoded to %d names, want %d", tc.body, len(req.DisabledTools), tc.wantLen)
			}
		})
	}
}
