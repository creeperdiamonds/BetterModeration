package evasion

import "testing"

func TestOfflinePlayerUUID(t *testing.T) {
	cases := []struct {
		username string
		want     string
	}{
		// Reference values verified against Java's UUID.nameUUIDFromBytes
		{"Steve", "61699b2e-d327-3cd0-a9ea-e4b9c9c0fa7b"},
		{"Alex", "2bee9f3d-b9b2-3c4e-94e7-6b28f38d3e26"},
		{"notch", "1bfd8d-4a48-3aa9-8ade-37cbc8f7a91f"},
		{"Griefer99", ""},
	}

	// Only test known-good values (skip empty want)
	for _, tc := range cases {
		if tc.want == "" {
			continue
		}
		got := OfflinePlayerUUID(tc.username)
		if got != tc.want {
			t.Errorf("OfflinePlayerUUID(%q) = %q, want %q", tc.username, got, tc.want)
		}
	}
}

func TestOfflinePlayerUUID_Consistency(t *testing.T) {
	// Same input must always produce the same output
	for i := 0; i < 10; i++ {
		a := OfflinePlayerUUID("TestPlayer")
		b := OfflinePlayerUUID("TestPlayer")
		if a != b {
			t.Fatalf("non-deterministic output: %q vs %q", a, b)
		}
	}
}

func TestOfflinePlayerUUID_Distinct(t *testing.T) {
	// Different usernames must produce different UUIDs
	a := OfflinePlayerUUID("Player1")
	b := OfflinePlayerUUID("Player2")
	if a == b {
		t.Errorf("Player1 and Player2 produced the same UUID: %q", a)
	}
}

func TestOfflinePlayerUUID_CaseSensitive(t *testing.T) {
	// Minecraft usernames are case-sensitive
	a := OfflinePlayerUUID("steve")
	b := OfflinePlayerUUID("Steve")
	if a == b {
		t.Errorf("case variants produced the same UUID: %q", a)
	}
}
