//go:build !grammar_subset || grammar_subset_arduino

package grammarruntime

// RegisterArduinoSupport registers the scanner support for arduino.
func RegisterArduinoSupport() {
	RegisterExternalScanner("arduino", ArduinoExternalScanner{})
}
