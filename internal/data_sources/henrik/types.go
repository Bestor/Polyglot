package henrik

// Wire-format structs for HenrikDev API responses (https://docs.henrikdev.xyz).
// Only fields we actually consume are modeled; unknown fields are ignored by
// encoding/json. Field names should be re-verified against a live response
// once a real API key is available, since HenrikDev's schema has shifted
// across versions and isn't formally versioned via a machine-readable spec.

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

type matchMetadata struct {
	MatchID string `json:"match_id"`
	Map     struct {
		Name string `json:"name"`
	} `json:"map"`
	Queue struct {
		ID string `json:"id"`
	} `json:"queue"`
	Mode         string `json:"mode"`
	SeasonID     string `json:"season_id"`
	StartedAt    string `json:"started_at"`
	RoundsPlayed int    `json:"rounds_played"`
}

type matchResponse struct {
	Status int `json:"status"`
	Data   struct {
		Metadata matchMetadata      `json:"metadata"`
		Players  []matchPlayerStats `json:"players"`
	} `json:"data"`
}

type matchPlayerStats struct {
	PUUID     string `json:"puuid"`
	Name      string `json:"name"`
	Tag       string `json:"tag"`
	TeamID    string `json:"team_id"`
	Character struct {
		Name string `json:"name"`
	} `json:"character"`
	Stats struct {
		Kills          int `json:"kills"`
		Deaths         int `json:"deaths"`
		Assists        int `json:"assists"`
		Headshots      int `json:"headshots"`
		Bodyshots      int `json:"bodyshots"`
		Legshots       int `json:"legshots"`
		DamageMade     int `json:"damage_made"`
		DamageReceived int `json:"damage_received"`
		Score          int `json:"score"`
	} `json:"stats"`
	Won bool `json:"won"`
}

type contentResponse struct {
	Seasons []struct {
		ID       string `json:"id"`
		ParentID string `json:"parentID"`
		IsActive bool   `json:"isActive"`
	} `json:"seasons"`
}
