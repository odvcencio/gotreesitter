// Package wasmfixtures pins the ASCII Go inputs used by the WASM runtime probe.
package wasmfixtures

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

type Fixture struct {
	Name   string
	Source []byte
	Edited []byte
	SHA256 string
}

func Cases() []Fixture {
	return []Fixture{
		sized("functions-64k", 65536, false, "787a5f02172d7d89be5f339e9653c3785e28d4004520489a5320dccb54323da5"),
		functions(4500),
		sized("functions-1m", 1048576, false, "66e6e2d95fc2b124fe7b5050dc3228d1ea01c79d10c47e47118a230a6146861d"),
		sized("nested-1m", 1048576, true, "91f2a2bbd6706b84f381429d4ffa3c71a84170ef00e489f5e2d3d45b816e2717"),
	}
}

func functions(count int) Fixture {
	var b strings.Builder
	b.WriteString("package main\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "func F%d() int { return %d }\n", i, i)
	}
	source := b.String()
	return checked("functions-4500", source,
		strings.Replace(source, "return 4499 }\n", "return 4498 }\n", 1),
		"a3e6f63a616dfb7c9ee1b739c9082e49066fd00d494fc6691cc4b713efdbe833")
}

func sized(name string, size int, deep bool, digest string) Fixture {
	var source, edited string
	if deep {
		source = "package main\nvar X = " + strings.Repeat("(", 12000) + "1" + strings.Repeat(")", 12000) + "\n"
		edited = strings.Replace(source, "1)", "2)", 1)
	} else {
		var b strings.Builder
		b.Grow(size)
		b.WriteString("package main\n")
		i := 0
		for {
			line := fmt.Sprintf("func F%d() int { return %d }\n", i, i)
			if b.Len()+len(line) > size-3 {
				break
			}
			b.WriteString(line)
			i++
		}
		source = b.String()
		edited = strings.Replace(source, fmt.Sprintf("return %d }\n", i-1), fmt.Sprintf("return %d }\n", i-2), 1)
	}
	padding := "//" + strings.Repeat(" ", size-len(source)-3) + "\n"
	return checked(name, source+padding, edited+padding, digest)
}

func checked(name, source, edited, digest string) Fixture {
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(source))); got != digest {
		panic("WASM fixture digest differs: " + name)
	}
	if len(source) != len(edited) || source == edited {
		panic("WASM fixture edit differs: " + name)
	}
	return Fixture{Name: name, Source: []byte(source), Edited: []byte(edited), SHA256: digest}
}
