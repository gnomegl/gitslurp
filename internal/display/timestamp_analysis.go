package display

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/utils"
)

func displayTimestampAnalysis(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) {
	targetCommits := make(map[string][]models.CommitInfo)

	for email, details := range emails {
		isTargetUser := userIdentifiers[email]
		if !isTargetUser {
			for name := range details.Names {
				if userIdentifiers[name] {
					isTargetUser = true
					break
				}
			}
		}

		if isTargetUser {
			for _, commits := range details.Commits {
				targetCommits[email] = append(targetCommits[email], commits...)
			}
		}
	}

	if len(targetCommits) == 0 {
		return
	}

	var allTargetCommits []models.CommitInfo
	for _, commits := range targetCommits {
		allTargetCommits = append(allTargetCommits, commits...)
	}

	patterns := utils.GetTimestampPatterns(allTargetCommits)

	fmt.Println()
	headerColor.Printf("TIMESTAMP ANALYSIS")
	fmt.Printf(" (%d commits)\n", patterns["total_commits"])
	fmt.Println(strings.Repeat("-", 40))

	displayGeneralPatterns(patterns)

	if len(allTargetCommits) >= 10 {
		fmt.Println()
		displayAggregatedHourlyGraph(patterns)
	}

	for email, commits := range targetCommits {
		if len(commits) >= 3 {
			displayUserTimestampAnalysis(email, commits)
		}
	}

	displaySuspiciousPatterns(allTargetCommits)
}

func displayGeneralPatterns(patterns map[string]interface{}) {
	if unusualPct, ok := patterns["unusual_hour_percentage"].(float64); ok && unusualPct > 0 {
		color.Yellow("Unusual hours (10pm-6am): %.1f%%", unusualPct)
	}

	if weekendPct, ok := patterns["weekend_percentage"].(float64); ok && weekendPct > 0 {
		color.Cyan("Weekend commits: %.1f%%", weekendPct)
	}

	if nightOwlPct, ok := patterns["night_owl_percentage"].(float64); ok && nightOwlPct > 10 {
		color.Blue("Night owl (10pm-2am): %.1f%%", nightOwlPct)
	}

	if earlyBirdPct, ok := patterns["early_bird_percentage"].(float64); ok && earlyBirdPct > 10 {
		color.Green("Early bird (5am-7am): %.1f%%", earlyBirdPct)
	}

	if mostActiveHour, ok := patterns["most_active_hour"].(int); ok {
		fmt.Printf("%s %02d:00\n", color.WhiteString("Most active hour:"), mostActiveHour)
	}

	if mostActiveDay, ok := patterns["most_active_day"].(time.Weekday); ok {
		fmt.Printf("%s %s\n", color.WhiteString("Most active day:"), mostActiveDay.String())
	}

	if mostActiveTZ, ok := patterns["most_active_timezone"].(string); ok && mostActiveTZ != "" {
		fmt.Printf("%s %s\n", color.WhiteString("Most common timezone:"), mostActiveTZ)
	}

	if tzDist, ok := patterns["timezone_distribution"].(map[string]int); ok && len(tzDist) > 1 {
		color.Yellow("Timezones: %d detected", len(tzDist))
		displayTimezoneDistribution(tzDist)
	}
}

func displayTimezoneDistribution(tzDist map[string]int) {
	type tzEntry struct {
		zone  string
		count int
	}

	var zones []tzEntry
	for tz, count := range tzDist {
		zones = append(zones, tzEntry{tz, count})
	}

	sort.Slice(zones, func(i, j int) bool {
		return zones[i].count > zones[j].count
	})

	for i, zone := range zones {
		if i >= 3 {
			break
		}
		fmt.Printf("  %s: %d commits\n", zone.zone, zone.count)
	}
}

func displayUserTimestampAnalysis(email string, commits []models.CommitInfo) {
	patterns := utils.GetTimestampPatterns(commits)

	fmt.Println()
	fmt.Printf("%s (%d commits):\n", color.WhiteString(email), len(commits))

	if mostActiveTZ, ok := patterns["most_active_timezone"].(string); ok && mostActiveTZ != "" {
		fmt.Printf("  Primary timezone: %s\n", mostActiveTZ)
	}

	if tzDist, ok := patterns["timezone_distribution"].(map[string]int); ok && len(tzDist) > 1 {
		color.Yellow("  Multiple timezones: %d zones detected", len(tzDist))
	}

	if unusualPct, ok := patterns["unusual_hour_percentage"].(float64); ok && unusualPct > 30 {
		color.Yellow("  %.1f%% unusual hour commits (in stated timezone)", unusualPct)
	}

	if nightOwlPct, ok := patterns["night_owl_percentage"].(float64); ok && nightOwlPct > 20 {
		color.Blue("  %.1f%% night owl pattern (10pm-2am local)", nightOwlPct)
	}

	if earlyBirdPct, ok := patterns["early_bird_percentage"].(float64); ok && earlyBirdPct > 20 {
		color.Green("  %.1f%% early bird pattern (5am-7am local)", earlyBirdPct)
	}

	if mostActiveHour, ok := patterns["most_active_hour"].(int); ok {
		fmt.Printf("  Most active: %02d:00 local time\n", mostActiveHour)
	}
}

func displayAggregatedHourlyGraph(patterns map[string]interface{}) {
	hourDist, ok := patterns["hour_distribution"].(map[int]int)
	if !ok || len(hourDist) == 0 {
		return
	}

	maxCommits := 0
	for _, count := range hourDist {
		if count > maxCommits {
			maxCommits = count
		}
	}

	if maxCommits == 0 {
		return
	}

	fmt.Printf("%s\n", color.WhiteString("Hourly activity:"))

	barWidth := 30

	for hour := 0; hour < 24; hour++ {
		count := hourDist[hour]
		barLen := 0
		if maxCommits > 0 {
			barLen = int(float64(count) / float64(maxCommits) * float64(barWidth))
		}
		bar := strings.Repeat("#", barLen)

		fmt.Printf("%02d:00 |", hour)
		if count > 0 {
			var colorFn func(format string, a ...interface{}) string
			switch {
			case hour >= 22 || hour <= 2:
				colorFn = color.RedString
			case hour >= 5 && hour <= 7:
				colorFn = color.GreenString
			case hour >= 9 && hour <= 17:
				colorFn = color.BlueString
			default:
				colorFn = color.YellowString
			}
			fmt.Printf("%s %d\n", colorFn("%-30s", bar), count)
		} else {
			fmt.Printf("%s\n", strings.Repeat(" ", barWidth))
		}
	}
}

func displaySuspiciousPatterns(commits []models.CommitInfo) {
	suspiciousCommits := make([]models.CommitInfo, 0)

	for _, commit := range commits {
		if commit.TimestampAnalysis != nil && commit.TimestampAnalysis.IsUnusualHour {
			suspiciousCommits = append(suspiciousCommits, commit)
		}
	}

	if len(suspiciousCommits) > 0 && len(suspiciousCommits) <= 15 {
		fmt.Println()
		fmt.Println("Unusual Hour Commits (Target Users):")

		sort.Slice(suspiciousCommits, func(i, j int) bool {
			return suspiciousCommits[i].AuthorDate.After(suspiciousCommits[j].AuthorDate)
		})

		for i, commit := range suspiciousCommits {
			if i >= 8 {
				break
			}

			localTimeStr := commit.AuthorDate.Format("2006-01-02 15:04:05")
			color.Yellow("  %s at %s (%s)", commit.Hash[:8], localTimeStr, commit.TimestampAnalysis.CommitTimezone)
			if commit.TimestampAnalysis.TimeZoneHint != "" {
				fmt.Printf("    %s\n", commit.TimestampAnalysis.TimeZoneHint)
			}
		}
	} else if len(suspiciousCommits) > 15 {
		fmt.Printf("\nFound %d unusual hour commits (showing pattern summary above)\n", len(suspiciousCommits))
	}
}
