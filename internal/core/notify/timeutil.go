package notify

import "time"

var shanghaiLocation = loadShanghaiLocation()

func loadShanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*3600)
	}
	return loc
}

func formatShanghaiRFC3339(t time.Time) string {
	return t.In(shanghaiLocation).Format(time.RFC3339)
}
