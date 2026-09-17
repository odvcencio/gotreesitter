package grammarruntime

import (
	"github.com/odvcencio/gotreesitter"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	RegisterCatalog(func(name string) ([]byte, func(), error) {
		data, err := os.ReadFile(filepath.Join("..", "grammar_blobs", name))
		return data, nil, err
	}, nil)
}
func BlobByName(name string) []byte {
	data, _ := os.ReadFile(filepath.Join("..", "grammar_blobs", strings.TrimSuffix(name, ".bin")+".bin"))
	return data
}
func AdaLanguage() *gotreesitter.Language            { return Language("ada") }
func ApexLanguage() *gotreesitter.Language           { return Language("apex") }
func AsmLanguage() *gotreesitter.Language            { return Language("asm") }
func AwkLanguage() *gotreesitter.Language            { return Language("awk") }
func BashLanguage() *gotreesitter.Language           { return Language("bash") }
func CLanguage() *gotreesitter.Language              { return Language("c") }
func CSharpLanguage() *gotreesitter.Language         { return Language("c_sharp") }
func CaddyLanguage() *gotreesitter.Language          { return Language("caddy") }
func CooklangLanguage() *gotreesitter.Language       { return Language("cooklang") }
func CppLanguage() *gotreesitter.Language            { return Language("cpp") }
func CrystalLanguage() *gotreesitter.Language        { return Language("crystal") }
func CssLanguage() *gotreesitter.Language            { return Language("css") }
func DLanguage() *gotreesitter.Language              { return Language("d") }
func DartLanguage() *gotreesitter.Language           { return Language("dart") }
func DhallLanguage() *gotreesitter.Language          { return Language("dhall") }
func DotLanguage() *gotreesitter.Language            { return Language("dot") }
func ElixirLanguage() *gotreesitter.Language         { return Language("elixir") }
func ErlangLanguage() *gotreesitter.Language         { return Language("erlang") }
func FortranLanguage() *gotreesitter.Language        { return Language("fortran") }
func FsharpLanguage() *gotreesitter.Language         { return Language("fsharp") }
func GoLanguage() *gotreesitter.Language             { return Language("go") }
func GomodLanguage() *gotreesitter.Language          { return Language("gomod") }
func GraphqlLanguage() *gotreesitter.Language        { return Language("graphql") }
func GroovyLanguage() *gotreesitter.Language         { return Language("groovy") }
func HackLanguage() *gotreesitter.Language           { return Language("hack") }
func HaskellLanguage() *gotreesitter.Language        { return Language("haskell") }
func HaxeLanguage() *gotreesitter.Language           { return Language("haxe") }
func HclLanguage() *gotreesitter.Language            { return Language("hcl") }
func HtmlLanguage() *gotreesitter.Language           { return Language("html") }
func HttpLanguage() *gotreesitter.Language           { return Language("http") }
func JavaLanguage() *gotreesitter.Language           { return Language("java") }
func JavascriptLanguage() *gotreesitter.Language     { return Language("javascript") }
func JsdocLanguage() *gotreesitter.Language          { return Language("jsdoc") }
func JsonLanguage() *gotreesitter.Language           { return Language("json") }
func KdlLanguage() *gotreesitter.Language            { return Language("kdl") }
func KotlinLanguage() *gotreesitter.Language         { return Language("kotlin") }
func LuaLanguage() *gotreesitter.Language            { return Language("lua") }
func MarkdownInlineLanguage() *gotreesitter.Language { return Language("markdown_inline") }
func MatlabLanguage() *gotreesitter.Language         { return Language("matlab") }
func MesonLanguage() *gotreesitter.Language          { return Language("meson") }
func ObjcLanguage() *gotreesitter.Language           { return Language("objc") }
func OdinLanguage() *gotreesitter.Language           { return Language("odin") }
func PerlLanguage() *gotreesitter.Language           { return Language("perl") }
func PythonLanguage() *gotreesitter.Language         { return Language("python") }
func RegoLanguage() *gotreesitter.Language           { return Language("rego") }
func RobotLanguage() *gotreesitter.Language          { return Language("robot") }
func RubyLanguage() *gotreesitter.Language           { return Language("ruby") }
func RustLanguage() *gotreesitter.Language           { return Language("rust") }
func ScalaLanguage() *gotreesitter.Language          { return Language("scala") }
func ScssLanguage() *gotreesitter.Language           { return Language("scss") }
func SwiftLanguage() *gotreesitter.Language          { return Language("swift") }
func TclLanguage() *gotreesitter.Language            { return Language("tcl") }
func TomlLanguage() *gotreesitter.Language           { return Language("toml") }
func TsxLanguage() *gotreesitter.Language            { return Language("tsx") }
func TypescriptLanguage() *gotreesitter.Language     { return Language("typescript") }
func TypstLanguage() *gotreesitter.Language          { return Language("typst") }
func UxntalLanguage() *gotreesitter.Language         { return Language("uxntal") }
func VLanguage() *gotreesitter.Language              { return Language("v") }
func YamlLanguage() *gotreesitter.Language           { return Language("yaml") }
