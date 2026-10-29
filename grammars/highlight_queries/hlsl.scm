; inherits: cpp
; Source: nvim-treesitter/nvim-treesitter runtime/queries/hlsl/highlights.scm at a2f694478be07f89c2d03cc17858a8745a400c4b (Apache-2.0)
; Change: none.

[
  "in"
  "out"
  "inout"
  "uniform"
  "shared"
  "groupshared"
  "discard"
  "cbuffer"
  "row_major"
  "column_major"
  "globallycoherent"
  "centroid"
  "noperspective"
  "nointerpolation"
  "sample"
  "linear"
  "snorm"
  "unorm"
  "point"
  "line"
  "triangleadj"
  "lineadj"
  "triangle"
] @keyword.modifier

((identifier) @variable.builtin
  (#lua-match? @variable.builtin "^SV_"))

(hlsl_attribute) @attribute

(hlsl_attribute
  [
    "["
    "]"
  ] @attribute)
