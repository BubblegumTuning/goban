package models

import "testing"

func TestGetColumnID_CaseAndAliases(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"todo", "todo-0"},
		{"Todo", "todo-0"},
		{"TODO", "todo-0"},
		{"To Do", "todo-0"},
		{"inprogress", "inprogress-0"},
		{"In Progress", "inprogress-0"},
		{"review", "review-0"},
		{"REVIEW-0", "review-0"},
		{"todo-0", "todo-0"},
		{"backlog", "backlog-0"},
		{"cancelled", "cancelled-0"},
		{"done", "done-0"},
		{"doing", "inprogress-0"}, // legacy name, not a live column
		{"doing-0", "inprogress-0"},
	}
	for _, tc := range cases {
		if got := GetColumnID(tc.in); got != tc.want {
			t.Errorf("GetColumnID(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
