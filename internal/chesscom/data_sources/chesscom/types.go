package chesscom

// Wire shapes for chess.com's Published Data API
// (https://www.chess.com/news/view/published-data-api), verified against
// live responses rather than documentation alone - a few fields don't
// match what the docs alone would suggest (see client.go's mapping
// comments, e.g. "eco" being an opening-name URL, not a short code).

type wireProfile struct {
	PlayerID  int64  `json:"player_id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Title     string `json:"title"`
	Country   string `json:"country"` // a URL, e.g. "https://api.chess.com/pub/country/US" - the trailing segment is the ISO code
	Followers int    `json:"followers"`
	Joined    int64  `json:"joined"` // unix seconds
	Status    string `json:"status"`
	Avatar    string `json:"avatar"`
}

type wireArchivesResponse struct {
	Archives []string `json:"archives"`
}

type wireGamePlayer struct {
	Username string `json:"username"` // preserves display casing, unlike the profile endpoint's lowercase username
	Rating   int    `json:"rating"`
	Result   string `json:"result"`
}

type wireAccuracies struct {
	White float64 `json:"white"`
	Black float64 `json:"black"`
}

type wireGame struct {
	URL         string          `json:"url"`
	PGN         string          `json:"pgn"`
	TimeControl string          `json:"time_control"`
	EndTime     int64           `json:"end_time"` // unix seconds - chess.com never reports a start time
	Rated       bool            `json:"rated"`
	UUID        string          `json:"uuid"`
	FEN         string          `json:"fen"`
	TimeClass   string          `json:"time_class"`
	Rules       string          `json:"rules"`
	White       wireGamePlayer  `json:"white"`
	Black       wireGamePlayer  `json:"black"`
	ECO         string          `json:"eco"` // an opening-name URL slug, not a 3-letter ECO code (that only lives in the PGN's own [ECO "..."] header, which this package doesn't parse)
	Accuracies  *wireAccuracies `json:"accuracies"`
}

type wireMonthGamesResponse struct {
	Games []wireGame `json:"games"`
}
