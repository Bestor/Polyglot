package henrik

// Wire-format structs for HenrikDev API responses
// (https://docs.henrikdev.xyz). Field names below were verified against a
// real /valorant/v4/match/{region}/{matchid} response captured during
// development - see git history for the raw sample. Only fields we
// actually consume are modeled; unknown fields are ignored by
// encoding/json.

type accountResponse struct {
	Status int `json:"status"`
	Data   struct {
		PUUID  string `json:"puuid"`
		Name   string `json:"name"`
		Tag    string `json:"tag"`
		Region string `json:"region"`
	} `json:"data"`
}

type matchListResponse struct {
	Status int `json:"status"`
	Data   []struct {
		Metadata matchMetadata `json:"metadata"`
	} `json:"data"`
}

type wireIDName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type matchMetadata struct {
	MatchID      string     `json:"match_id"`
	Map          wireIDName `json:"map"`
	GameVersion  string     `json:"game_version"`
	GameLengthMs int        `json:"game_length_in_ms"`
	StartedAt    string     `json:"started_at"`
	IsCompleted  bool       `json:"is_completed"`
	Queue        struct {
		ID       string `json:"id"`
		ModeType string `json:"mode_type"`
	} `json:"queue"`
	Season struct {
		ID    string `json:"id"`
		Short string `json:"short"`
	} `json:"season"`
	Platform string `json:"platform"`
	Region   string `json:"region"`
	Cluster  string `json:"cluster"`
}

type matchResponse struct {
	Status int `json:"status"`
	Data   struct {
		Metadata matchMetadata    `json:"metadata"`
		Players  []matchPlayer    `json:"players"`
		Teams    []matchTeam      `json:"teams"`
		Rounds   []matchRound     `json:"rounds"`
		Kills    []matchKillEvent `json:"kills"`
	} `json:"data"`
}

type wirePlayerRef struct {
	PUUID string `json:"puuid"`
	Name  string `json:"name"`
	Tag   string `json:"tag"`
	Team  string `json:"team"`
}

type wireLocation struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type wirePlayerLocation struct {
	Player      wirePlayerRef `json:"player"`
	ViewRadians float64       `json:"view_radians"`
	Location    wireLocation  `json:"location"`
}

type wireWeapon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type matchPlayer struct {
	PUUID    string     `json:"puuid"`
	Name     string     `json:"name"`
	Tag      string     `json:"tag"`
	TeamID   string     `json:"team_id"`
	Platform string     `json:"platform"`
	PartyID  string     `json:"party_id"`
	Agent    wireIDName `json:"agent"`
	Stats    struct {
		Score     int `json:"score"`
		Kills     int `json:"kills"`
		Deaths    int `json:"deaths"`
		Assists   int `json:"assists"`
		Headshots int `json:"headshots"`
		Bodyshots int `json:"bodyshots"`
		Legshots  int `json:"legshots"`
		Damage    struct {
			Dealt    int `json:"dealt"`
			Received int `json:"received"`
		} `json:"damage"`
	} `json:"stats"`
	AbilityCasts struct {
		Grenade  int `json:"grenade"`
		Ability1 int `json:"ability1"`
		Ability2 int `json:"ability2"`
		Ultimate int `json:"ultimate"`
	} `json:"ability_casts"`
	Tier struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"tier"`
	AccountLevel      int `json:"account_level"`
	SessionPlaytimeMs int `json:"session_playtime_in_ms"`
	Behavior          struct {
		AFKRounds    float64 `json:"afk_rounds"`
		FriendlyFire struct {
			Incoming float64 `json:"incoming"`
			Outgoing float64 `json:"outgoing"`
		} `json:"friendly_fire"`
		RoundsInSpawn float64 `json:"rounds_in_spawn"`
	} `json:"behavior"`
	Economy struct {
		Spent struct {
			Overall int     `json:"overall"`
			Average float64 `json:"average"`
		} `json:"spent"`
		LoadoutValue struct {
			Overall int     `json:"overall"`
			Average float64 `json:"average"`
		} `json:"loadout_value"`
	} `json:"economy"`
}

type matchTeam struct {
	TeamID string `json:"team_id"`
	Rounds struct {
		Won  int `json:"won"`
		Lost int `json:"lost"`
	} `json:"rounds"`
	Won bool `json:"won"`
}

type matchDamageEvent struct {
	Player    wirePlayerRef `json:"player"` // the victim, from the attacker's perspective
	Bodyshots int           `json:"bodyshots"`
	Headshots int           `json:"headshots"`
	Legshots  int           `json:"legshots"`
	Damage    int           `json:"damage"`
}

type matchRoundPlayerStat struct {
	Player       wirePlayerRef      `json:"player"`
	DamageEvents []matchDamageEvent `json:"damage_events"`
	Stats        struct {
		Score     int `json:"score"`
		Kills     int `json:"kills"`
		Headshots int `json:"headshots"`
		Bodyshots int `json:"bodyshots"`
		Legshots  int `json:"legshots"`
	} `json:"stats"`
	Economy struct {
		LoadoutValue int         `json:"loadout_value"`
		Remaining    int         `json:"remaining"`
		Weapon       wireWeapon  `json:"weapon"`
		Armor        *wireIDName `json:"armor"`
	} `json:"economy"`
	WasAFK          bool `json:"was_afk"`
	ReceivedPenalty bool `json:"received_penalty"`
	StayedInSpawn   bool `json:"stayed_in_spawn"`
}

type matchPlantEvent struct {
	RoundTimeMs     int                  `json:"round_time_in_ms"`
	Site            string               `json:"site"`
	Location        wireLocation         `json:"location"`
	Player          wirePlayerRef        `json:"player"`
	PlayerLocations []wirePlayerLocation `json:"player_locations"`
}

type matchDefuseEvent struct {
	RoundTimeMs     int                  `json:"round_time_in_ms"`
	Location        wireLocation         `json:"location"`
	Player          wirePlayerRef        `json:"player"`
	PlayerLocations []wirePlayerLocation `json:"player_locations"`
}

type matchRound struct {
	ID          int                    `json:"id"` // 0-indexed round number
	Result      string                 `json:"result"`
	Ceremony    string                 `json:"ceremony"`
	WinningTeam string                 `json:"winning_team"`
	Plant       *matchPlantEvent       `json:"plant"`
	Defuse      *matchDefuseEvent      `json:"defuse"`
	Stats       []matchRoundPlayerStat `json:"stats"`
}

type matchKillEvent struct {
	TimeInRoundMs     int                  `json:"time_in_round_in_ms"`
	TimeInMatchMs     int                  `json:"time_in_match_in_ms"`
	Round             int                  `json:"round"`
	Killer            wirePlayerRef        `json:"killer"`
	Victim            wirePlayerRef        `json:"victim"`
	Assistants        []wirePlayerRef      `json:"assistants"`
	Location          wireLocation         `json:"location"`
	Weapon            wireWeapon           `json:"weapon"`
	SecondaryFireMode bool                 `json:"secondary_fire_mode"`
	PlayerLocations   []wirePlayerLocation `json:"player_locations"`
}

// contentResponse models /valorant/v1/content. Confusingly, HenrikDev calls
// seasons/acts "acts" here (both episodes and their child acts show up in
// the same flat list, disambiguated by Type) - there is no "seasons" key,
// verified against a real response captured during development.
type contentResponse struct {
	Data struct {
		Acts []struct {
			ID       string `json:"id"`
			ParentID string `json:"parentId"`
			Type     string `json:"type"` // "episode" | "act"
			Name     string `json:"name"` // e.g. "V26", "ACT VI" - short, human-readable
			IsActive bool   `json:"isActive"`
		} `json:"acts"`
	} `json:"data"`
}
