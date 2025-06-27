package utils

import (
	"time"

	"git.sr.ht/~gnome/gitslurp/internal/models"
)

func AnalyzeTimestamp(commitTime time.Time) *models.TimestampAnalysis {
	utcTime := commitTime.UTC()
	localTime := commitTime
	
	analysis := &models.TimestampAnalysis{
		HourOfDay:      commitTime.Hour(),
		LocalHourOfDay: commitTime.Hour(),
		DayOfWeek:      commitTime.Weekday(),
		UTCTime:        utcTime,
		LocalTime:      localTime,
		CommitTimezone: commitTime.Location().String(),
	}

	analysis.IsWeekend = analysis.DayOfWeek == time.Saturday || analysis.DayOfWeek == time.Sunday

	analysis.IsUnusualHour = analysis.LocalHourOfDay <= 5 || analysis.LocalHourOfDay >= 22
	analysis.IsNightOwl = analysis.LocalHourOfDay >= 22 || analysis.LocalHourOfDay <= 2
	analysis.IsEarlyBird = analysis.LocalHourOfDay >= 5 && analysis.LocalHourOfDay <= 7

	if analysis.IsUnusualHour {
		if analysis.IsNightOwl {
			analysis.TimeZoneHint = "Night owl activity in " + analysis.CommitTimezone
		} else {
			analysis.TimeZoneHint = "Early morning activity in " + analysis.CommitTimezone
		}
	} else {
		analysis.TimeZoneHint = "Normal hours in " + analysis.CommitTimezone
	}

	return analysis
}

func GetTimestampPatterns(commits []models.CommitInfo) map[string]interface{} {
	patterns := make(map[string]interface{})
	
	hourDistribution := make(map[int]int)
	dayDistribution := make(map[time.Weekday]int)
	timezoneDistribution := make(map[string]int)
	unusualHourCount := 0
	weekendCount := 0
	nightOwlCount := 0
	earlyBirdCount := 0

	for _, commit := range commits {
		if commit.TimestampAnalysis != nil {
			hourDistribution[commit.TimestampAnalysis.LocalHourOfDay]++
			dayDistribution[commit.TimestampAnalysis.DayOfWeek]++
			timezoneDistribution[commit.TimestampAnalysis.CommitTimezone]++
			
			if commit.TimestampAnalysis.IsUnusualHour {
				unusualHourCount++
			}
			if commit.TimestampAnalysis.IsWeekend {
				weekendCount++
			}
			if commit.TimestampAnalysis.IsNightOwl {
				nightOwlCount++
			}
			if commit.TimestampAnalysis.IsEarlyBird {
				earlyBirdCount++
			}
		}
	}

	totalCommits := len(commits)
	if totalCommits > 0 {
		patterns["unusual_hour_percentage"] = float64(unusualHourCount) / float64(totalCommits) * 100
		patterns["weekend_percentage"] = float64(weekendCount) / float64(totalCommits) * 100
		patterns["night_owl_percentage"] = float64(nightOwlCount) / float64(totalCommits) * 100
		patterns["early_bird_percentage"] = float64(earlyBirdCount) / float64(totalCommits) * 100
	}

	patterns["hour_distribution"] = hourDistribution
	patterns["day_distribution"] = dayDistribution
	patterns["timezone_distribution"] = timezoneDistribution
	patterns["total_commits"] = totalCommits

	mostActiveHour := findMostActiveHour(hourDistribution)
	mostActiveDay := findMostActiveDay(dayDistribution)
	mostActiveTimezone := findMostActiveTimezone(timezoneDistribution)
	
	patterns["most_active_hour"] = mostActiveHour
	patterns["most_active_day"] = mostActiveDay
	patterns["most_active_timezone"] = mostActiveTimezone

	return patterns
}

func findMostActiveHour(hourDist map[int]int) int {
	maxCount := 0
	mostActive := 0
	
	for hour, count := range hourDist {
		if count > maxCount {
			maxCount = count
			mostActive = hour
		}
	}
	
	return mostActive
}

func findMostActiveDay(dayDist map[time.Weekday]int) time.Weekday {
	maxCount := 0
	var mostActive time.Weekday
	
	for day, count := range dayDist {
		if count > maxCount {
			maxCount = count
			mostActive = day
		}
	}
	
	return mostActive
}

func findMostActiveTimezone(tzDist map[string]int) string {
	maxCount := 0
	mostActive := ""
	
	for tz, count := range tzDist {
		if count > maxCount {
			maxCount = count
			mostActive = tz
		}
	}
	
	return mostActive
}
