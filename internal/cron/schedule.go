package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const cronFieldCount = 5

type parsedSchedule struct {
	minutes cronField
	hours   cronField
	dom     cronField
	months  cronField
	dow     cronField
}

type cronField struct {
	allowed []bool
	min     int
	max     int
}

func NextRunAfter(spec string, after time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty schedule")
	}
	if strings.HasPrefix(trimmed, "@") {
		return nextSpecialRunAfter(trimmed, after)
	}
	parsed, err := parseSchedule(trimmed)
	if err != nil {
		return time.Time{}, err
	}
	location := after.Location()
	current := after.In(location).Truncate(time.Minute).Add(time.Minute)
	deadline := current.AddDate(5, 0, 0)
	for !current.After(deadline) {
		if parsed.matches(current) {
			return current, nil
		}
		current = current.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no run time found for schedule %q within 5 years", spec)
}

func MatchesSchedule(spec string, at time.Time) (bool, error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return false, fmt.Errorf("empty schedule")
	}
	if strings.HasPrefix(trimmed, "@") {
		return matchesSpecialSchedule(trimmed, at)
	}
	parsed, err := parseSchedule(trimmed)
	if err != nil {
		return false, err
	}
	return parsed.matches(at), nil
}

func nextSpecialRunAfter(spec string, after time.Time) (time.Time, error) {
	switch strings.TrimSpace(spec) {
	case "@hourly":
		base := after.Truncate(time.Hour).Add(time.Hour)
		return time.Date(base.Year(), base.Month(), base.Day(), base.Hour(), 0, 0, 0, after.Location()), nil
	case "@daily":
		base := after.In(after.Location()).Truncate(24 * time.Hour).Add(24 * time.Hour)
		return time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, after.Location()), nil
	case "@weekly":
		base := after.In(after.Location()).Truncate(24 * time.Hour).Add(24 * time.Hour)
		for base.Weekday() != time.Monday {
			base = base.Add(24 * time.Hour)
		}
		return time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, after.Location()), nil
	case "@monthly":
		base := after.In(after.Location())
		year, month, _ := base.Date()
		month++
		if month > time.December {
			month = time.January
			year++
		}
		return time.Date(year, month, 1, 0, 0, 0, 0, after.Location()), nil
	default:
		if !strings.HasPrefix(spec, "@every ") {
			return time.Time{}, fmt.Errorf("unsupported schedule %q", spec)
		}
		duration, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(spec, "@every ")))
		if err != nil || duration <= 0 {
			return time.Time{}, fmt.Errorf("invalid @every duration in %q", spec)
		}
		return after.Add(duration), nil
	}
}

func matchesSpecialSchedule(spec string, at time.Time) (bool, error) {
	switch strings.TrimSpace(spec) {
	case "@hourly":
		return at.Minute() == 0 && at.Second() == 0, nil
	case "@daily":
		return at.Hour() == 0 && at.Minute() == 0 && at.Second() == 0, nil
	case "@weekly":
		return at.Weekday() == time.Monday && at.Hour() == 0 && at.Minute() == 0 && at.Second() == 0, nil
	case "@monthly":
		return at.Day() == 1 && at.Hour() == 0 && at.Minute() == 0 && at.Second() == 0, nil
	default:
		if strings.HasPrefix(spec, "@every ") {
			return false, nil
		}
		return false, fmt.Errorf("unsupported schedule %q", spec)
	}
}

func parseSchedule(spec string) (parsedSchedule, error) {
	parts := strings.Fields(spec)
	if len(parts) != cronFieldCount {
		return parsedSchedule{}, fmt.Errorf("invalid cron schedule %q: expected 5 fields", spec)
	}
	minutes, err := parseCronField(parts[0], 0, 59, false)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("invalid minute field: %w", err)
	}
	hours, err := parseCronField(parts[1], 0, 23, false)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("invalid hour field: %w", err)
	}
	dom, err := parseCronField(parts[2], 1, 31, false)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("invalid day-of-month field: %w", err)
	}
	months, err := parseCronField(parts[3], 1, 12, false)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("invalid month field: %w", err)
	}
	dow, err := parseCronField(parts[4], 0, 6, true)
	if err != nil {
		return parsedSchedule{}, fmt.Errorf("invalid day-of-week field: %w", err)
	}
	return parsedSchedule{minutes: minutes, hours: hours, dom: dom, months: months, dow: dow}, nil
}

func parseCronField(spec string, minValue, maxValue int, allowSevenAsSunday bool) (cronField, error) {
	field := cronField{allowed: make([]bool, maxValue-minValue+1), min: minValue, max: maxValue}
	for _, token := range strings.Split(strings.TrimSpace(spec), ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return cronField{}, fmt.Errorf("empty token")
		}
		if err := field.addToken(token, allowSevenAsSunday); err != nil {
			return cronField{}, err
		}
	}
	return field, nil
}

func (f cronField) addToken(token string, allowSevenAsSunday bool) error {
	base := token
	step := 1
	if strings.Contains(token, "/") {
		parts := strings.Split(token, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid step token %q", token)
		}
		base = strings.TrimSpace(parts[0])
		parsedStep, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || parsedStep <= 0 {
			return fmt.Errorf("invalid step in %q", token)
		}
		step = parsedStep
	}
	start := f.min
	end := f.max
	switch {
	case base == "*":
		// keep defaults
	case strings.Contains(base, "-"):
		parts := strings.Split(base, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid range %q", token)
		}
		parsedStart, err := parseCronNumber(parts[0], f.min, f.max, allowSevenAsSunday)
		if err != nil {
			return err
		}
		parsedEnd, err := parseCronNumber(parts[1], f.min, f.max, allowSevenAsSunday)
		if err != nil {
			return err
		}
		start, end = parsedStart, parsedEnd
	default:
		value, err := parseCronNumber(base, f.min, f.max, allowSevenAsSunday)
		if err != nil {
			return err
		}
		start, end = value, value
	}
	if start > end {
		return fmt.Errorf("range start %d exceeds end %d", start, end)
	}
	for value := start; value <= end; value += step {
		f.allowed[value-f.min] = true
	}
	return nil
}

func parseCronNumber(raw string, minValue, maxValue int, allowSevenAsSunday bool) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	if allowSevenAsSunday && value == 7 {
		value = 0
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf("value %d out of range [%d,%d]", value, minValue, maxValue)
	}
	return value, nil
}

func (s parsedSchedule) matches(t time.Time) bool {
	return s.minutes.matches(t.Minute()) &&
		s.hours.matches(t.Hour()) &&
		s.dom.matches(t.Day()) &&
		s.months.matches(int(t.Month())) &&
		s.dow.matches(int(t.Weekday()))
}

func (f cronField) matches(value int) bool {
	if value < f.min || value > f.max {
		return false
	}
	return f.allowed[value-f.min]
}
