package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type TimeOfDay struct {
	Hour   int
	Minute int
	Second int
}

type Window struct {
	Start        TimeOfDay
	RolloutStart TimeOfDay
	End          TimeOfDay
	Location     *time.Location
}

func New(start, rolloutStart, end, timezone string) (Window, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Window{}, err
	}
	startAt, err := parseTimeOfDay(start)
	if err != nil {
		return Window{}, fmt.Errorf("invalid update window start: %w", err)
	}
	rolloutAt, err := parseTimeOfDay(rolloutStart)
	if err != nil {
		return Window{}, fmt.Errorf("invalid rollout start: %w", err)
	}
	endAt, err := parseTimeOfDay(end)
	if err != nil {
		return Window{}, fmt.Errorf("invalid update window end: %w", err)
	}
	return Window{
		Start:        startAt,
		RolloutStart: rolloutAt,
		End:          endAt,
		Location:     loc,
	}, nil
}

func (w Window) IsOpen(now time.Time) bool {
	start, end := w.bounds(now)
	return !now.Before(start) && now.Before(end)
}

func (w Window) Remaining(now time.Time) time.Duration {
	start, end := w.bounds(now)
	if now.Before(start) || !now.Before(end) {
		return 0
	}
	return end.Sub(now)
}

func (w Window) CurrentDate(now time.Time) string {
	return now.In(w.Location).Format("2006-01-02")
}

func (w Window) bounds(now time.Time) (time.Time, time.Time) {
	localNow := now.In(w.Location)
	year, month, day := localNow.Date()

	start := at(w.Location, year, month, day, w.Start)
	rolloutStart := at(w.Location, year, month, day, w.RolloutStart)
	end := at(w.Location, year, month, day, w.End)

	if !end.After(start) {
		end = end.Add(24 * time.Hour)
		if localNow.Before(end) && localNow.Hour() < w.End.Hour {
			start = start.Add(-24 * time.Hour)
			rolloutStart = rolloutStart.Add(-24 * time.Hour)
		}
	}

	if rolloutStart.Before(start) {
		rolloutStart = start
	}
	if rolloutStart.After(start) {
		start = rolloutStart
	}

	return start, end
}

func at(loc *time.Location, year int, month time.Month, day int, tod TimeOfDay) time.Time {
	return time.Date(year, month, day, tod.Hour, tod.Minute, tod.Second, 0, loc)
}

func parseTimeOfDay(value string) (TimeOfDay, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return TimeOfDay{}, fmt.Errorf("expected HH:MM or HH:MM:SS")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return TimeOfDay{}, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return TimeOfDay{}, err
	}
	second := 0
	if len(parts) == 3 {
		second, err = strconv.Atoi(parts[2])
		if err != nil {
			return TimeOfDay{}, err
		}
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return TimeOfDay{}, fmt.Errorf("time out of range")
	}
	return TimeOfDay{Hour: hour, Minute: minute, Second: second}, nil
}
