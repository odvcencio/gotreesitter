//go:build !grammar_blobs_external && !grammar_set_core && !grammar_subset

package grammars

import "embed"

//go:embed grammar_blobs/*.bin
var grammarBlobFS embed.FS

func readGrammarBlob(blobName string) (grammarBlob, error) {
	data, err := grammarBlobFS.ReadFile("grammar_blobs/" + blobName)
	if err != nil {
		return grammarBlob{}, err
	}
	return grammarBlob{data: data}, nil
}
