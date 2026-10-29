package crawl

import (
	"errors"
	"time"
)

// Frequencies of the crawl.
const (
	Weekly  = "weekly"
	Monthly = "monthly"
)

// Schedule is when the crawl runs, as the data screen sets it.
type Schedule struct {
	Enabled  bool   `json:"enabled"`
	Every    string `json:"every"`     // weekly or monthly
	Weekday  int    `json:"weekday"`   // weekly: 0 Sunday … 6 Saturday
	Day      int    `json:"day"`       // monthly: 1-28, so every month has it
	Hour     int    `json:"hour"`      // 0-23, server time
	MaxPages int    `json:"max_pages"` // catalogue pages walked per run
}

// DefaultSchedule runs on the 1st of every month at 02:00, when nobody is
// asking (the owner's choice, 2026-10-29). Twenty pages cover a month of new
// documents with room to spare.
func DefaultSchedule() Schedule {
	return Schedule{Enabled: true, Every: Monthly, Weekday: int(time.Sunday), Day: 1, Hour: 2, MaxPages: 20}
}

// Validate refuses a schedule the scheduler cannot follow.
func (s Schedule) Validate() error {
	switch {
	case s.Every != Weekly && s.Every != Monthly:
		return errors.New("every must be weekly or monthly")
	case s.Weekday < 0 || s.Weekday > 6:
		return errors.New("weekday must be 0 (Sunday) to 6")
	case s.Day < 1 || s.Day > 28:
		return errors.New("day must be 1-28")
	case s.Hour < 0 || s.Hour > 23:
		return errors.New("hour must be 0-23")
	case s.MaxPages < 1 || s.MaxPages > 200:
		return errors.New("max_pages must be 1-200")
	}
	return nil
}

// RecheckParts is how many runs it takes to fetch every in-scope document
// again: a quarter a week, so all within a month either way.
func (s Schedule) RecheckParts() int {
	if s.Every == Monthly {
		return 1
	}
	return 4
}

// LastSlot is the latest time at or before now the schedule names. A crawl
// is due when none has been queued since.
func (s Schedule) LastSlot(now time.Time) time.Time {
	if s.Every == Monthly {
		slot := time.Date(now.Year(), now.Month(), s.Day, s.Hour, 0, 0, 0, now.Location())
		if slot.After(now) {
			slot = slot.AddDate(0, -1, 0)
		}
		return slot
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, 0, 0, 0, now.Location())
	back := (int(now.Weekday()) - s.Weekday + 7) % 7
	slot := day.AddDate(0, 0, -back)
	if slot.After(now) {
		slot = slot.AddDate(0, 0, -7)
	}
	return slot
}
