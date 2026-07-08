package grammargen

import "testing"

func TestCobolCorpusSnippetDeepParity(t *testing.T) {
	if raceEnabled {
		t.Skip("skip heavyweight COBOL parity generation under -race; non-race coverage keeps the generated-vs-reference check")
	}

	assertImportedDeepParityCases(t, "cobol", []struct {
		name string
		src  string
	}{
		{
			name: "working_storage_picture_x_ranges",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       data division.\n" +
				"       working-storage section.\n" +
				"       01 str1 PIC X(10)xx(111).\n" +
				"       01 str2 PIC X(10).\n" +
				"       01 str2 PIC X.\n",
		},
		{
			name: "close_no_rewind_statement",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f assign PRINTER.\n" +
				"       procedure division.\n" +
				"       close f no rewind. \n",
		},
		{
			name: "close_with_no_rewind_statement",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f assign PRINTER.\n" +
				"       procedure division.\n" +
				"       close f with no rewind. \n",
		},
		{
			name: "close_with_lock_statement",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f assign PRINTER.\n" +
				"       procedure division.\n" +
				"       close f with lock. \n",
		},
		{
			name: "close_unit_removal_statement",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f assign PRINTER.\n" +
				"       procedure division.\n" +
				"       close f unit removal.\n",
		},
		{
			name: "close_unit_for_removal_statement",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f assign PRINTER.\n" +
				"       procedure division.\n" +
				"       close f unit for removal.\n",
		},
		{
			name: "open_extend_statement",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f assign PRINTER.\n" +
				"       procedure division.\n" +
				"       open extend f.\n",
		},
		{
			name: "pic_x_corpus_block_exact",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       data division.\n" +
				"       working-storage section.\n" +
				"       01 str1 PIC X(10)xx(111).\n" +
				"       01 str2 PIC X(10).\n" +
				"       01 str2 PIC X.\n" +
				"       procedure division.\n",
		},
		{
			name: "select_alternate_record_with_duplicates_corpus_block_exact",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       environment division.\n" +
				"       input-output section.\n" +
				"       FILE-CONTROL.\n" +
				"       select f alternate record a of b with duplicates.\n" +
				"       data division.\n" +
				"       working-storage section.\n" +
				"       01 b.\n" +
				"           03 a PIC X.\n",
		},
		{
			name: "perform_label_until_corpus_block_exact",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       data division.\n" +
				"       working-storage section.\n" +
				"       01 c PIC 9.\n" +
				"       procedure division.\n" +
				"       perform aa varying c FROM 1 BY 1 UNTIL c >= 1.\n" +
				"       aa.\n",
		},
		{
			name: "perform_forever_block_corpus_block_exact",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"       perform forever\n" +
				"         continue\n" +
				"       end-perform.",
		},
		{
			name: "evaluate_when_other_corpus_block_exact",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"       evaluate 1\n" +
				"       when 1\n" +
				"         go to aa\n" +
				"       when 2\n" +
				"         go to aa\n" +
				"       when other\n" +
				"         go to aa\n" +
				"       end-evaluate.\n" +
				"       aa.\n",
		},
		{
			name: "fixed_format_comment_corpus_block_exact",
			src: "aaaaaa identification division.\n" +
				"aaaaaa program-id. a.  ,,, ;;;                                          aaaaa\n" +
				"      *aaaa\n",
		},
	})
}
