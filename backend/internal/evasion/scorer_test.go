package evasion

import (
	"testing"
)

func TestScore_Allow(t *testing.T) {
	result := Score(Signals{})
	if result.Action != ActionAllow {
		t.Errorf("empty signals: want ALLOW, got %s (score %d)", result.Action, result.Score)
	}
	if result.Score != 0 {
		t.Errorf("empty signals: want score 0, got %d", result.Score)
	}
	if len(result.Flags) != 0 {
		t.Errorf("empty signals: want no flags, got %v", result.Flags)
	}
}

func TestScore_FlagThreshold(t *testing.T) {
	// VPN (40) + SubnetMatch (15) + NewAccount (10) = 65 → FLAG
	result := Score(Signals{IsVPN: true, SubnetMatch: true, BrandNewAccount: true})
	if result.Action != ActionFlag {
		t.Errorf("want FLAG, got %s (score %d)", result.Action, result.Score)
	}
}

func TestScore_DenyThreshold(t *testing.T) {
	// IPBanned (50) + VPN (40) = 90 → DENY
	result := Score(Signals{IPBanned: true, IsVPN: true})
	if result.Action != ActionDeny {
		t.Errorf("want DENY, got %s (score %d)", result.Action, result.Score)
	}
}

func TestScore_OfflineUUIDAloneDenies(t *testing.T) {
	// OfflineUUIDMatch (60) + UsernameExact (55) = 115 → capped at 100 → DENY
	result := Score(Signals{OfflineUUIDMatch: true, UsernameExact: true})
	if result.Action != ActionDeny {
		t.Errorf("want DENY, got %s (score %d)", result.Action, result.Score)
	}
	if result.Score != 100 {
		t.Errorf("want capped score 100, got %d", result.Score)
	}
}

func TestScore_OfflineUUIDAloneFlags(t *testing.T) {
	// OfflineUUIDMatch alone = 60 → exactly FLAG
	result := Score(Signals{OfflineUUIDMatch: true})
	if result.Action != ActionFlag {
		t.Errorf("want FLAG, got %s (score %d)", result.Action, result.Score)
	}
}

func TestScore_FlagsPresent(t *testing.T) {
	result := Score(Signals{IsVPN: true, IPBanned: true})
	found := map[string]bool{}
	for _, f := range result.Flags {
		found[f] = true
	}
	if !found[FlagVPNIP] {
		t.Error("expected VPN_IP flag")
	}
	if !found[FlagIPBanned] {
		t.Error("expected IP_BANNED flag")
	}
}

func TestScore_HostingHalfWeight(t *testing.T) {
	// Hosting alone = 20 (half of ScoreVPN=40) → below FLAG, ALLOW
	result := Score(Signals{IsHosting: true})
	if result.Action != ActionAllow {
		t.Errorf("hosting alone: want ALLOW, got %s (score %d)", result.Action, result.Score)
	}
	if result.Score != ScoreVPN/2 {
		t.Errorf("hosting alone: want score %d, got %d", ScoreVPN/2, result.Score)
	}
}

func TestScore_HostingWithVPNNotDoubled(t *testing.T) {
	// IsVPN=true already triggers VPN flag; IsHosting=true should not add extra
	withVPN := Score(Signals{IsVPN: true})
	withVPNAndHosting := Score(Signals{IsVPN: true, IsHosting: true})
	if withVPN.Score != withVPNAndHosting.Score {
		t.Errorf("hosting should not add extra score when VPN is already set: %d vs %d",
			withVPN.Score, withVPNAndHosting.Score)
	}
}
