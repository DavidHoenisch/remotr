package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type InhibitionPolicy string

const (
	InhibitionDefer  InhibitionPolicy = "defer"
	InhibitionIgnore InhibitionPolicy = "ignore"
)

func (p InhibitionPolicy) Valid() bool { return p == InhibitionDefer || p == InhibitionIgnore }

type RebootMaintenanceWindow struct {
	Weekdays []string `yaml:"weekdays"`
	Start    string   `yaml:"start"`
	Duration string   `yaml:"duration"`
}

func (r *RebootResource) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if generation := strings.TrimSpace(r.Generation); generation == "" || len(generation) > 256 {
		return fmt.Errorf("generation is required and must not exceed 256 bytes")
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent {
		return fmt.Errorf("lifecycle %q is unsupported; reboot resources are intent-only", r.Lifecycle)
	}
	if r.Risk != "" && r.Risk != RiskBoot && r.Risk != RiskDestructive {
		return fmt.Errorf("risk %q cannot reduce reboot's boot safety class", r.Risk)
	}
	if r.Delay != "" {
		delay, err := time.ParseDuration(r.Delay)
		if err != nil || delay < 0 || delay > 24*time.Hour {
			return fmt.Errorf("delay must be between 0 and 24h")
		}
	}
	timeout, err := time.ParseDuration(r.Timeout)
	if err != nil || timeout <= 0 || timeout > time.Hour {
		return fmt.Errorf("timeout must be between 1ns and 1h")
	}
	if r.Deadline != "" {
		if _, err := time.Parse(time.RFC3339, r.Deadline); err != nil {
			return fmt.Errorf("deadline must be RFC3339: %w", err)
		}
	}
	if r.UserInhibition == "" {
		r.UserInhibition = InhibitionDefer
	}
	if !r.UserInhibition.Valid() {
		return fmt.Errorf("userInhibition must be defer or ignore")
	}
	if r.WorkloadInhibition == "" {
		r.WorkloadInhibition = InhibitionDefer
	}
	if !r.WorkloadInhibition.Valid() {
		return fmt.Errorf("workloadInhibition must be defer or ignore")
	}
	if r.MaintenanceWindow != nil {
		if err := r.MaintenanceWindow.Validate(); err != nil {
			return fmt.Errorf("maintenanceWindow: %w", err)
		}
	}
	return nil
}

func (w RebootMaintenanceWindow) Validate() error {
	if len(w.Weekdays) == 0 {
		return fmt.Errorf("weekdays are required")
	}
	for _, weekday := range w.Weekdays {
		if _, ok := parseWeekday(weekday); !ok {
			return fmt.Errorf("weekdays contains unknown day %q", weekday)
		}
	}
	if _, err := w.StartMinute(); err != nil {
		return err
	}
	duration, err := time.ParseDuration(w.Duration)
	if err != nil || duration <= 0 || duration > 7*24*time.Hour {
		return fmt.Errorf("duration must be between 1ns and 168h")
	}
	return nil
}

func (w RebootMaintenanceWindow) StartMinute() (int, error) {
	parts := strings.Split(w.Start, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("start must use HH:MM UTC")
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || len(parts[0]) != 2 || len(parts[1]) != 2 || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("start must use HH:MM UTC")
	}
	return hour*60 + minute, nil
}

func (w RebootMaintenanceWindow) Contains(at time.Time) bool {
	startMinute, err := w.StartMinute()
	if err != nil {
		return false
	}
	duration, err := time.ParseDuration(w.Duration)
	if err != nil {
		return false
	}
	at = at.UTC()
	for dayOffset := 0; dayOffset >= -7; dayOffset-- {
		day := at.AddDate(0, 0, dayOffset)
		if !w.includes(day.Weekday()) {
			continue
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(startMinute) * time.Minute)
		if !at.Before(start) && at.Before(start.Add(duration)) {
			return true
		}
	}
	return false
}

func (w RebootMaintenanceWindow) includes(day time.Weekday) bool {
	for _, value := range w.Weekdays {
		parsed, ok := parseWeekday(value)
		if ok && parsed == day {
			return true
		}
	}
	return false
}

func parseWeekday(value string) (time.Weekday, bool) {
	for day := time.Sunday; day <= time.Saturday; day++ {
		if strings.EqualFold(value, day.String()) {
			return day, true
		}
	}
	return 0, false
}

func (r RebootResource) DelayDuration() time.Duration {
	d, _ := time.ParseDuration(r.Delay)
	return d
}

func (r RebootResource) TimeoutDuration() time.Duration {
	d, _ := time.ParseDuration(r.Timeout)
	return d
}

func (r RebootResource) DeadlineTime() time.Time {
	if r.Deadline == "" {
		return time.Time{}
	}
	d, _ := time.Parse(time.RFC3339, r.Deadline)
	return d
}
