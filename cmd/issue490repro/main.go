package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	debug.SetMaxStack(64 << 20)

	root := filepath.Join(runtime.GOROOT(), "src", "net")
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	lang := grammars.GoLanguage()
	pool := gotreesitter.NewParserPool(lang)

	var files int
	total := time.Now()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		fmt.Printf("%03d parsing %-50s ", files+1, filepath.Base(path))
		start := time.Now()
		tree, err := pool.Parse(src)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return nil
		}
		fmt.Printf("%v\n", time.Since(start))
		tree.Release()
		files++
		return nil
	})
	if err != nil {
		fmt.Printf("walk error: %v\n", err)
	}

	fmt.Printf("\n%d files in %v\n", files, time.Since(total))
}
