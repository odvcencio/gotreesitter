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
