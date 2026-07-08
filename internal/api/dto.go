package api

type AskRequest struct {
	Question string `json:"question"`
}

type AskResponse struct {
	Answer        string `json:"answer"`
	MatchesSynced int    `json:"matches_synced"`
}

type WarmRequest struct {
	Name  string `json:"name"`
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type WarmResponse struct {
	PUUID   string `json:"puuid"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	Region  string `json:"region"`
	Fetched int    `json:"fetched"`
	Skipped int    `json:"skipped"`
}
