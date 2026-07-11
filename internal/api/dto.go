package api

type AskRequest struct {
	Question string `json:"question"`
}

type AskResponse struct {
	Answer        string `json:"answer"`
	MatchesSynced int    `json:"matches_synced"`
}

type WarmRequest struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
	// Count bounds how many matches to fetch. Ignored when All is true.
	Count int `json:"count"`
	// All, when true, loads the player's entire match history instead of
	// being bounded by Count - this can take a while under the upstream
	// rate limit for a player with a long history.
	All bool `json:"all"`
}

type WarmResponse struct {
	PUUID   string `json:"puuid"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Region  string `json:"region"`
	Fetched int    `json:"fetched"`
	Skipped int    `json:"skipped"`
	// HistoryExhausted is true if this call reached the true end of the
	// player's match history - only realistically expected when All was
	// set. Once true, later /api/ask questions about this player never
	// need to re-check the upstream API for older matches, however far
	// back they ask.
	HistoryExhausted bool `json:"history_exhausted"`
}
