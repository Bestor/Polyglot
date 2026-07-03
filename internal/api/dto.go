package api

type PlayerRef struct {
	Name   string `json:"name"`
	Tag    string `json:"tag"`
	Region string `json:"region"`
}

type AskRequest struct {
	Question string      `json:"question"`
	Players  []PlayerRef `json:"players"`
}

type AskResponse struct {
	Answer        string `json:"answer"`
	MatchesSynced int    `json:"matches_synced"`
}
