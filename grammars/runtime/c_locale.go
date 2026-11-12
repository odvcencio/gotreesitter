package grammarruntime

// Character classes of the C locale, which the pinned C scanners see when they
// call iswspace and iswalnum: only ASCII characters belong to a class.

// cIsSpace is iswspace in the C locale: space, tab, line feed, vertical tab,
// form feed and carriage return.
func cIsSpace(ch rune) bool {
	return ch == ' ' || (ch >= '\t' && ch <= '\r')
}

// cIsAlnum is iswalnum in the C locale: ASCII letters and digits.
func cIsAlnum(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

// cIsDigit is iswdigit in the C locale.
func cIsDigit(ch rune) bool { return ch >= '0' && ch <= '9' }

// cIsAlpha is iswalpha in the C locale: ASCII letters only.
func cIsAlpha(ch rune) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
