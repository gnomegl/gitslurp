package network

import (
	"sort"
	"strings"
	"time"
)

type PatternAnalyzer struct {
	minSupport float64
}

func NewPatternAnalyzer() *PatternAnalyzer {
	return &PatternAnalyzer{
		minSupport: 0.1,
	}
}

type CollaborationPattern struct {
	Users        []string
	Frequency    int
	Repositories []string
	TimeSpan     time.Duration
	Type         string
}

func (p *PatternAnalyzer) FindFrequentCollaborators(network *UserNetwork) []CollaborationPattern {
	patterns := make([]CollaborationPattern, 0)

	collaboratorPairs := make(map[string]map[string]int)
	for username, collab := range network.Collaborators {
		if _, exists := collaboratorPairs[username]; !exists {
			collaboratorPairs[username] = make(map[string]int)
		}

		for _, repo := range collab.Repositories {
			collaboratorPairs[username][repo]++
		}
	}

	for user1, repos1 := range collaboratorPairs {
		for user2, repos2 := range collaboratorPairs {
			if user1 >= user2 {
				continue
			}

			sharedRepos := p.findSharedRepos(repos1, repos2)
			if len(sharedRepos) >= 2 {
				patterns = append(patterns, CollaborationPattern{
					Users:        []string{user1, user2},
					Frequency:    len(sharedRepos),
					Repositories: sharedRepos,
					Type:         "frequent_pair",
				})
			}
		}
	}

	return patterns
}

func (p *PatternAnalyzer) findSharedRepos(repos1, repos2 map[string]int) []string {
	shared := make([]string, 0)
	for repo := range repos1 {
		if _, exists := repos2[repo]; exists {
			shared = append(shared, repo)
		}
	}
	return shared
}

type TeamDetector struct {
	minTeamSize int
	minOverlap  float64
}

func NewTeamDetector() *TeamDetector {
	return &TeamDetector{
		minTeamSize: 3,
		minOverlap:  0.5,
	}
}

func (t *TeamDetector) DetectTeams(network *UserNetwork) []Team {
	teams := make([]Team, 0)

	repoCollaborators := make(map[string][]string)
	for username, collab := range network.Collaborators {
		for _, repo := range collab.Repositories {
			repoCollaborators[repo] = append(repoCollaborators[repo], username)
		}
	}

	processedGroups := make(map[string]bool)

	for repo, collaborators := range repoCollaborators {
		if len(collaborators) < t.minTeamSize {
			continue
		}

		sort.Strings(collaborators)
		groupKey := strings.Join(collaborators, ",")

		if processedGroups[groupKey] {
			continue
		}
		processedGroups[groupKey] = true

		sharedRepos := t.findAllSharedRepos(collaborators, network)

		if float64(len(sharedRepos))/float64(len(collaborators)) >= t.minOverlap {
			team := Team{
				Members:      collaborators,
				Repositories: sharedRepos,
				Size:         len(collaborators),
				Cohesion:     t.calculateCohesion(collaborators, network),
			}
			teams = append(teams, team)
		}
	}

	sort.Slice(teams, func(i, j int) bool {
		return teams[i].Cohesion > teams[j].Cohesion
	})

	return teams
}

func (t *TeamDetector) findAllSharedRepos(members []string, network *UserNetwork) []string {
	if len(members) == 0 {
		return nil
	}

	repoCount := make(map[string]int)
	for _, member := range members {
		if collab, exists := network.Collaborators[member]; exists {
			for _, repo := range collab.Repositories {
				repoCount[repo]++
			}
		}
	}

	shared := make([]string, 0)
	for repo, count := range repoCount {
		if count == len(members) {
			shared = append(shared, repo)
		}
	}

	return shared
}

func (t *TeamDetector) calculateCohesion(members []string, network *UserNetwork) float64 {
	if len(members) <= 1 {
		return 0.0
	}

	totalConnections := 0
	mutualConnections := 0

	for i, member1 := range members {
		for j, member2 := range members {
			if i >= j {
				continue
			}

			totalConnections++

			if contains(network.MutualConnections, member1) && contains(network.MutualConnections, member2) {
				mutualConnections++
			}
		}
	}

	if totalConnections == 0 {
		return 0.0
	}

	return float64(mutualConnections) / float64(totalConnections)
}

type Team struct {
	Members      []string
	Repositories []string
	Size         int
	Cohesion     float64
}

type ActivityAnalyzer struct{}

func NewActivityAnalyzer() *ActivityAnalyzer {
	return &ActivityAnalyzer{}
}

func (a *ActivityAnalyzer) AnalyzeCollaborationTimeline(coAuthors map[string]*CoAuthorInfo) *CollaborationTimeline {
	timeline := &CollaborationTimeline{
		Events:        make([]TimelineEvent, 0),
		ActivePeriods: make([]ActivePeriod, 0),
	}

	for email, coAuthor := range coAuthors {
		timeline.Events = append(timeline.Events, TimelineEvent{
			Date:        coAuthor.FirstCommit,
			Type:        "collaboration_start",
			Participant: coAuthor.Name,
			Email:       email,
			Repository:  coAuthor.Repositories[0],
		})

		if coAuthor.CommitCount > 10 {
			timeline.Events = append(timeline.Events, TimelineEvent{
				Date:        coAuthor.LastCommit,
				Type:        "frequent_collaborator",
				Participant: coAuthor.Name,
				Email:       email,
				Count:       coAuthor.CommitCount,
			})
		}
	}

	sort.Slice(timeline.Events, func(i, j int) bool {
		return timeline.Events[i].Date.Before(timeline.Events[j].Date)
	})

	timeline.ActivePeriods = a.identifyActivePeriods(timeline.Events)

	return timeline
}

func (a *ActivityAnalyzer) identifyActivePeriods(events []TimelineEvent) []ActivePeriod {
	if len(events) == 0 {
		return nil
	}

	periods := make([]ActivePeriod, 0)
	currentPeriod := ActivePeriod{
		Start:  events[0].Date,
		End:    events[0].Date,
		Events: 1,
	}

	for i := 1; i < len(events); i++ {
		if events[i].Date.Sub(currentPeriod.End) <= 30*24*time.Hour {
			currentPeriod.End = events[i].Date
			currentPeriod.Events++
		} else {
			if currentPeriod.Events >= 3 {
				periods = append(periods, currentPeriod)
			}
			currentPeriod = ActivePeriod{
				Start:  events[i].Date,
				End:    events[i].Date,
				Events: 1,
			}
		}
	}

	if currentPeriod.Events >= 3 {
		periods = append(periods, currentPeriod)
	}

	return periods
}

type CollaborationTimeline struct {
	Events        []TimelineEvent
	ActivePeriods []ActivePeriod
}

type TimelineEvent struct {
	Date        time.Time
	Type        string
	Participant string
	Email       string
	Repository  string
	Count       int
}

type ActivePeriod struct {
	Start  time.Time
	End    time.Time
	Events int
}

type InfluenceCalculator struct{}

func NewInfluenceCalculator() *InfluenceCalculator {
	return &InfluenceCalculator{}
}

func (i *InfluenceCalculator) CalculateInfluence(network *UserNetwork) map[string]float64 {
	influence := make(map[string]float64)

	followerScore := float64(len(network.Followers)) * 0.001
	if followerScore > 0.3 {
		followerScore = 0.3
	}
	influence["follower_influence"] = followerScore

	followingRatio := 0.0
	if len(network.Following) > 0 {
		followingRatio = float64(len(network.Followers)) / float64(len(network.Following))
		if followingRatio > 10 {
			influence["following_ratio"] = 0.3
		} else if followingRatio > 5 {
			influence["following_ratio"] = 0.2
		} else if followingRatio > 2 {
			influence["following_ratio"] = 0.1
		}
	}

	collaboratorScore := float64(len(network.Collaborators)) * 0.02
	if collaboratorScore > 0.3 {
		collaboratorScore = 0.3
	}
	influence["collaborator_influence"] = collaboratorScore

	totalRepos := 0
	for _, collab := range network.Collaborators {
		totalRepos += len(collab.Repositories)
	}
	repoScore := float64(totalRepos) * 0.005
	if repoScore > 0.2 {
		repoScore = 0.2
	}
	influence["repository_influence"] = repoScore

	orgScore := float64(len(network.Organizations)) * 0.1
	if orgScore > 0.3 {
		orgScore = 0.3
	}
	influence["organization_influence"] = orgScore

	totalInfluence := 0.0
	for _, score := range influence {
		totalInfluence += score
	}
	if totalInfluence > 1.0 {
		totalInfluence = 1.0
	}
	influence["total"] = totalInfluence

	return influence
}

