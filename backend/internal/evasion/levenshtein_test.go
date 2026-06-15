package evasion

import "testing"

func TestLevenshtein(t *testing.T) {
	cases := []struct{ a, b string; want int }{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"Saturday", "Sunday", 3},
		{"Player1", "Player2", 1},
		{"Griefer99", "Griefer9", 1},
		{"Griefer99", "Griefer999", 1},
		{"Griefer99", "Griefr99", 1},
		{"Steve", "steve", 1},
		{"abc", "xyz", 3},
		{"ban_evader", "ban_evadr", 1},
	}

	for _, tc := range cases {
		got := Levenshtein(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLevenshtein_Symmetric(t *testing.T) {
	pairs := [][2]string{
		{"kitten", "sitting"},
		{"Player123", "Player124"},
		{"Griefer", "Greifer"},
	}
	for _, p := range pairs {
		ab := Levenshtein(p[0], p[1])
		ba := Levenshtein(p[1], p[0])
		if ab != ba {
			t.Errorf("Levenshtein(%q,%q)=%d != Levenshtein(%q,%q)=%d", p[0], p[1], ab, p[1], p[0], ba)
		}
	}
}
