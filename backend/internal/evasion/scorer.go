package evasion

// Score weights for each signal.
const (
	ScoreVPN                = 40
	ScoreIPBanned           = 50
	ScoreASNMatch           = 25
	ScoreOfflineUUIDMatch   = 60
	ScoreUsernameExact      = 55
	ScoreUsernameLevenshtein = 35
	ScoreUsernamePrefix     = 20
	ScoreSubnetMatch        = 15
	ScoreNewAccount         = 10
	ScoreTimeCorrelation    = 30

	ThresholdFlag = 60
	ThresholdDeny = 85
)

// Action values returned in ConnectResult.
const (
	ActionAllow = "ALLOW"
	ActionFlag  = "FLAG"
	ActionDeny  = "DENY"
)

// Flag string constants included in the response and Redis event.
const (
	FlagVPNIP                = "VPN_IP"
	FlagHostingIP            = "HOSTING_IP"
	FlagIPBanned             = "IP_BANNED"
	FlagASNMatch             = "ASN_MATCH"
	FlagOfflineUUIDMatch     = "OFFLINE_UUID_MATCH"
	FlagUsernameExactBanned  = "USERNAME_EXACT_BANNED"
	FlagUsernameSimilar      = "USERNAME_SIMILAR_TO_BANNED"
	FlagUsernamePrefixBanned = "USERNAME_PREFIX_BANNED"
	FlagSubnetMatch          = "SUBNET_MATCH"
	FlagNewAccount           = "NEW_ACCOUNT"
	FlagTimeCorrelation      = "TIME_CORRELATION"
)

// Signals is the pre-assembled input to Score. The caller (service.go) builds
// this by querying the DB; Score itself makes no DB or network calls.
type Signals struct {
	IsVPN               bool // from ip_metadata
	IsProxy             bool // from ip_metadata
	IsTor               bool // from ip_metadata
	IsHosting           bool // from ip_metadata
	IPBanned            bool // IP linked to active BAN in org
	ASNMatchesBanned    bool // same ASN as a banned profile in org
	OfflineUUIDMatch    bool // joining UUID found in banned_offline_uuids
	UsernameExact       bool // exact username match to a banned profile's known username
	UsernameLevenshtein bool // Levenshtein ≤ 2 to banned username AND brand-new account
	UsernamePrefix      bool // shares ≥6-char prefix with a banned username
	SubnetMatch         bool // /24 subnet shared with a banned IP in org
	BrandNewAccount     bool // zero prior join_events in this org
	TimeCorrelation     bool // joined within 10 min of a ban being issued on the same IP
}

// ScoreResult is the output of Score.
type ScoreResult struct {
	Score  int
	Flags  []string
	Action string // "ALLOW" | "FLAG" | "DENY"
}

// Score computes a suspicion score from the provided signals and returns the
// recommended action. It is a pure function with no side effects.
func Score(s Signals) ScoreResult {
	score := 0
	var flags []string

	add := func(points int, flag string) {
		score += points
		flags = append(flags, flag)
	}

	if s.IsVPN || s.IsTor {
		add(ScoreVPN, FlagVPNIP)
	} else if s.IsProxy {
		add(ScoreVPN, FlagVPNIP)
	}
	if s.IsHosting && !s.IsVPN && !s.IsProxy && !s.IsTor {
		add(ScoreVPN/2, FlagHostingIP) // hosting alone is weaker signal
	}
	if s.IPBanned {
		add(ScoreIPBanned, FlagIPBanned)
	}
	if s.ASNMatchesBanned {
		add(ScoreASNMatch, FlagASNMatch)
	}
	if s.OfflineUUIDMatch {
		add(ScoreOfflineUUIDMatch, FlagOfflineUUIDMatch)
	}
	if s.UsernameExact {
		add(ScoreUsernameExact, FlagUsernameExactBanned)
	}
	if s.UsernameLevenshtein {
		add(ScoreUsernameLevenshtein, FlagUsernameSimilar)
	}
	if s.UsernamePrefix {
		add(ScoreUsernamePrefix, FlagUsernamePrefixBanned)
	}
	if s.SubnetMatch {
		add(ScoreSubnetMatch, FlagSubnetMatch)
	}
	if s.BrandNewAccount {
		add(ScoreNewAccount, FlagNewAccount)
	}
	if s.TimeCorrelation {
		add(ScoreTimeCorrelation, FlagTimeCorrelation)
	}

	if score > 100 {
		score = 100
	}

	action := ActionAllow
	if score >= ThresholdDeny {
		action = ActionDeny
	} else if score >= ThresholdFlag {
		action = ActionFlag
	}

	return ScoreResult{Score: score, Flags: flags, Action: action}
}
