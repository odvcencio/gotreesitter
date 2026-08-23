#!/bin/sh
set -e
cd /home/draco/work/gts-campaign-20260823
B=campaign/fixtures/b1
mkdir -p $(find campaign/fixtures -maxdepth 0); for d in beancount chatito commonlisp crystal csv cylc desktop disassembly earthfile editorconfig fish git_config git_rebase gitcommit hcl hyprlang kconfig liquid make matlab mermaid nushell odin pascal purescript prolog requirements tcl tmux twig uxntal v vimdoc xml; do mkdir -p "$B/$d"; done

cat > $B/beancount/sample.beancount <<'EOF'
2026-07-01 * "Grocery run"
  Expenses:Food:Groceries      42.17 USD
  Assets:Checking

2026-07-03 open Assets:Checking
2026-07-05 pad Assets:Checking Equity:Opening-Balances
2026-07-31 balance Assets:Checking 1204.55 USD
EOF

cat > $B/chatito/sample.chatito <<'EOF'
intent orderDrink
training order a latte
training can I get an espresso
@
    drink [latte] @id(1)
EOF

cat > $B/commonlisp/sample.lisp <<'EOF'
(defpackage #:census
  (:use #:cl)
  (:export #:classify))
(in-package #:census)

(defun classify (status)
  "Map a raw status keyword to its census bucket."
  (case status
    ((pass) :pass)
    ((fallback skip) :decline)
    (t :unknown)))
EOF

cat > $B/crystal/sample.cr <<'EOF'
class Census
  property counts = Hash(String, Int32).new(0)

  def bump(lang : String)
    @counts[lang] += 1
  end

  def total : Int32
    @counts.values.sum
  end
end

puts Census.new.tap { |c| c.bump("crystal") }.total
EOF

cat > $B/csv/sample.csv <<'EOF'
language,status,mechanism,bytes
go,PASS,,12438
bash,FALLBACK,zero-width-shift,6239
xml,FALLBACK,repetition-shift-class,480
EOF

cat > $B/cylc/sample.cylc <<'EOF'
[scheduling]
    initial cycle point = 20260701T00Z
    [[graph]]
        R1 = prep => census => report

[runtime]
    [[census]]
        script = python3 run_census.py --full
EOF

cat > $B/desktop/sample.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Census Runner
Exec=/usr/local/bin/run-census --full
Icon=utilities-terminal
Terminal=true
Categories=Development;
EOF

cat > $B/disassembly/sample.disassembly <<'EOF'
0000000000401000 <classify>:
  401000:	55                   	push   %rbp
  401001:	48 89 e5             	mov    %rsp,%rbp
  401004:	89 f8                	mov    %edi,%eax
  401006:	83 f8 01             	cmp    $0x1,%eax
  401009:	74 05                	je     401010 <pass>
  40100b:	b8 02 00 00 00       	mov    $0x2,%eax
  401010:	5d                   	pop    %rbp
  401011:	c3                   	retq
EOF

cat > $B/earthfile/Earthfile <<'EOF'
VERSION 0.8
FROM alpine:3.19
build:
    RUN apk add go git
    COPY . .
    RUN go build ./...
save-artifact census
EOF

cat > $B/editorconfig/sample.editorconfig <<'EOF'
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true

[*.go]
indent_style = tab
indent_size = 4
EOF

cat > $B/fish/sample.fish <<'EOF'
function census
    set -l langs (ls grammars | string match -r '^[a-z]+')
    for lang in $langs
        echo "checking $lang"
    end
end
census | grep checking | wc -l
EOF

cat > $B/git_config/sample.gitconfig <<'EOF'
[user]
	name = Draco
	email = draco@example.com
[core]
	editor = vim
	autocrlf = input
[alias]
	lg = log --oneline --graph -20
EOF

cat > $B/git_rebase/sample.gitrebase <<'EOF'
pick af9ded2b Merge pull request #897
pick 7a43c9cb Add admission census classifier
squash 41d0e77 Fix trailing-newline frontier offset
pick 9c21aa0 Update coverage docs
EOF

cat > $B/gitcommit/sample.commitmsg <<'EOF'
admission: add fine-grained decline mechanism census

Split the two coarse FALLBACK buckets into eleven mechanism classes,
gated behind GTS_ADMISSION_CENSUS=1 so every default-build call site
stays byte-identical.

PASS=48 DIVERGE=0 FALLBACK=153 SKIP=5 ERROR=0 total=206
EOF

cat > $B/hcl/main.hcl <<'EOF'
variable "region" {
  type    = string
  default = "us-east-1"
}

resource "aws_s3_bucket" "corpus" {
  bucket = "depth-census-${var.region}"
  tags = {
    owner = "census"
    tier  = "evidence"
  }
}
EOF

cat > $B/hyprlang/sample.conf <<'EOF'
monitor=eDP-1,1920x1080@60,0x0,1
input {
    kb_layout = us
    repeat_rate = 40
}
bind = SUPER, Return, exec, kitty
EOF

cat > $B/kconfig/Kconfig.census <<'EOF'
menu "Census options"

config GTS_ADMISSION_CENSUS
	bool "Enable fine-grained admission decline census"
	default n
	help
	  Tag every compact-route decline with its mechanism class.

config GTS_MAX_FRONTIER
	int "Maximum scheduler frontier size"
	range 64 4096
	default 512

endmenu
EOF

cat > $B/liquid/sample.liquid <<'EOF'
{% assign passing = site.languages | where: "status", "PASS" %}
<h2>{{ passing.size }} languages pass</h2>
<ul>
{% for lang in passing %}
  <li>{{ lang.name }} — {{ lang.mechanism | default: "byte-exact" }}</li>
{% endfor %}
</ul>
EOF

cat > $B/make/Makefile <<'EOF'
CENSUS ?= full
TAGS := gts_parsercorephase0

.PHONY: census
census:
	GTS_ADMISSION_SCORECARD=1 GTS_ADMISSION_CENSUS=1 \
	go test -tags $(TAGS) -run TestAdmissionCandidateScorecard206 -v .

clean:
	rm -rf campaign/out
EOF

cat > $B/matlab/classify.m <<'EOF'
function out = classify(status)
%CLASSIFY Map a raw status to its census bucket.
switch lower(status)
    case 'pass'
        out = 'PASS';
    case {'fallback', 'skip'}
        out = 'DECLINE';
    otherwise
        out = 'UNKNOWN';
end
end
EOF

cat > $B/mermaid/sample.mmd <<'EOF'
graph TD
    A[Smoke fixture] --> B{Compact route}
    B -->|accept| C[PASS]
    B -->|decline| D[FALLBACK]
    D --> E[repetition-shift-class]
    D --> F[zero-width-shift]
    D --> G[eof-byte-short-frontier]
EOF

cat > $B/nushell/sample.nu <<'EOF'
def census [] {
    ls campaign/fixtures/b1
    | where type == dir
    | each { |row| $row.name }
    | length
}
census
EOF

cat > $B/odin/main.odin <<'EOF'
package census

import "core:fmt"

classify :: proc(status: Status) -> string {
	switch status {
	case .Pass:    return "PASS"
	case .Fallack: return "FALLBACK"
	case .Skip:    return "SKIP"
	}
	return "UNKNOWN"
}

main :: proc() { fmt.println(classify(.Pass)) }
EOF

cat > $B/pascal/census.pas <<'EOF'
program Census;
type
  TStatus = (stPass, stFallback, stSkip);
var
  s: TStatus;
begin
  for s := Low(TStatus) to High(TStatus) do
    WriteLn(Ord(s));
end.
EOF

cat > $B/purescript/Main.purs <<'EOF'
module Census.Main where

import Prelude

classify :: String -> String
classify = case _ of
  "pass" -> "PASS"
  "skip" -> "SKIP"
  _ -> "FALLBACK"
EOF

cat > $B/prolog/census.pl <<'EOF'
:- module(census, [classify/2]).

classify(pass, pass).
classify(fallback, decline).
classify(skip, decline).
classify(_, unknown).
EOF

cat > $B/requirements/requirements.txt <<'EOF'
# pinned for the depth-census runner
pytest==8.2.0
pyyaml==6.0.1
tree-sitter==0.22.3
EOF

cat > $B/tcl/census.tcl <<'EOF'
proc classify {status} {
    switch -exact -- $status {
        pass      { return PASS }
        fallback  -
        skip      { return DECLINE }
        default   { return UNKNOWN }
    }
}
puts [classify pass]
EOF

cat > $B/tmux/sample.tmux.conf <<'EOF'
set -g base-index 1
setw -g aggressive-resize on
bind r source-file ~/.tmux.conf \; display "reloaded"
set -g status-style bg=black,fg=cyan
EOF

cat > $B/twig/sample.twig <<'EOF'
{% extends "layout.html.twig" %}
{% block body %}
<table class="census">
{% for lang in languages %}
  <tr><td>{{ lang.name }}</td><td>{{ lang.depthStatus }}</td></tr>
{% endfor %}
</table>
{% endblock %}
EOF

cat > $B/uxntal/census.tal <<'EOF'
( classify: 0=PASS 1=FALLBACK )
|0100 @on-reset
    #01 classify
    #18 DEO
    BRK

@classify ( status -- class )
    #01 AND JMP2r
EOF

cat > $B/v/main.v <<'EOF'
module census

struct Language {
	name   string
	status string
}

fn classify(status string) string {
	return match status {
		'pass' { 'PASS' }
		'skip' { 'SKIP' }
		else   { 'FALLBACK' }
	}
}

fn main() { println(classify('pass')) }
EOF

cat > $B/vimdoc/census.txt <<'EOF'
*census.txt*  Fine-grained decline mechanism census

INTRO                                    *census-intro*
The admission scorecard sorts all 206 registered grammars into five
buckets. See |scorecard| for the command.

COMMANDS                                 *census-commands*
:CensusRun       Run the full 206-language scorecard.
:CensusDepth     Rerun currently-PASSING languages on real corpora.

vim:tw=78:ts=8:noet:ft=help:norl:
EOF

cat > $B/xml/corpus-entry.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<census date="2026-08-23">
  <bucket name="PASS" count="48"/>
  <bucket name="FALLBACK" count="153">
    <mechanism name="eof-byte-short-frontier" count="90"/>
    <mechanism name="zero-width-shift" count="33"/>
    <mechanism name="repetition-shift-class" count="27"/>
  </bucket>
</census>
EOF

echo done
find $B -type f | wc -l