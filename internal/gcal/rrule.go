package gcal

import (
	"fmt"
	"strings"

	"github.com/Pedro-0101/gix-server/internal/recur"
)

func recurToRRULE(rule recur.Rule) string {
	if rule.Freq == "" {
		return ""
	}
	interval := rule.Interval
	if interval < 1 {
		interval = 1
	}

	var parts []string
	switch rule.Freq {
	case "daily":
		parts = []string{"FREQ=DAILY"}
	case "weekly":
		parts = []string{"FREQ=WEEKLY"}
		if rule.Weekday != "" {
			day := googleWeekday(rule.Weekday)
			if day != "" {
				parts = append(parts, "BYDAY="+day)
			}
		}
	case "monthly":
		parts = []string{"FREQ=MONTHLY"}
	case "yearly":
		parts = []string{"FREQ=YEARLY"}
	default:
		return ""
	}
	if interval > 1 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", interval))
	}
	return "RRULE:" + strings.Join(parts, ";")
}

func googleWeekday(day string) string {
	switch strings.ToUpper(day) {
	case "MON":
		return "MO"
	case "TUE":
		return "TU"
	case "WED":
		return "WE"
	case "THU":
		return "TH"
	case "FRI":
		return "FR"
	case "SAT":
		return "SA"
	case "SUN":
		return "SU"
	default:
		return ""
	}
}
