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
