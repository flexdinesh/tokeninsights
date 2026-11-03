package sqlutil

import "testing"

func TestBindPreservesQuotedQuestionMarks(t *testing.T) {
	input := `SELECT '?', 'it''s ?', "question?", "escaped""?", ? WHERE value=?`
	want := `SELECT '?', 'it''s ?', "question?", "escaped""?", $1 WHERE value=$2`
	if got := Bind(input); got != want {
		t.Fatal(got)
	}
}
