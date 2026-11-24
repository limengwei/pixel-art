package translate

import "testing"

func TestSubjectLookup(t *testing.T) {
	for _, tc := range []struct{ en, want string }{
		{"butterfly", "蝴蝶"},
		{"aa battery", "五号电池"},
		{"zweihander", "双手大剑"},
		{" Butterfly ", "蝴蝶"},
		{"no such subject xyz", ""},
	} {
		if got := Subject(tc.en); got != tc.want {
			t.Errorf("Subject(%q) = %q, want %q", tc.en, got, tc.want)
		}
	}
	if n := len(subjectMap()); n < 6000 {
		t.Errorf("merged subject map has %d entries, want >= 6000", n)
	}
}
