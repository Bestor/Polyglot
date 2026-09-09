package chesscom

import "testing"

func TestParseFlexibleDate(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"bare date", "2026-05-01", false},
		{"rfc3339", "2026-05-01T00:00:00Z", false},
		{"garbage", "not-a-date", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseFlexibleDate(c.input)
			if (err != nil) != c.wantErr {
				t.Errorf("parseFlexibleDate(%q) error = %v, wantErr %v", c.input, err, c.wantErr)
			}
		})
	}
}
