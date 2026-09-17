//go:build !grammar_subset || grammar_subset_comment

package grammarruntime

// RegisterCommentSupport registers the scanner support for comment.
func RegisterCommentSupport() {
	RegisterExternalScanner("comment", CommentExternalScanner{})
}
