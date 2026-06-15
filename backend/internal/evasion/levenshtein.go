package evasion

// Levenshtein returns the edit distance between a and b.
// Uses two-row rolling DP: O(min(|a|,|b|)) space, O(|a|*|b|) time.
// Operates on runes to handle multi-byte characters correctly.
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)

	// Keep ra as the longer string to minimise the inner loop length.
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}

	if len(rb) == 0 {
		return len(ra)
	}

	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)

	for j := range prev {
		prev[j] = j
	}

	for i, ca := range ra {
		curr[0] = i + 1
		for j, cb := range rb {
			ins := curr[j] + 1
			del := prev[j+1] + 1
			sub := prev[j]
			if ca != cb {
				sub++
			}
			curr[j+1] = min3(ins, del, sub)
		}
		prev, curr = curr, prev
	}

	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
