package search

import (
	"fmt"
	"sort"
	"strings"
)

type IdentityCorrelator struct {
	threshold float64
}

func NewIdentityCorrelator() *IdentityCorrelator {
	return &IdentityCorrelator{
		threshold: 0.6,
	}
}

type CorrelatedIdentity struct {
	PrimaryIdentifier string
	Type              string
	RelatedIdentities map[string][]string
	Confidence        float64
	Sources           []string
}

func (c *IdentityCorrelator) CorrelateIdentities(searchResult *ComprehensiveSearchResult) []CorrelatedIdentity {
	identities := make(map[string]*CorrelatedIdentity)

	for identifier, sources := range searchResult.Correlations {
		if len(sources) < 2 {
			continue
		}

		identity := &CorrelatedIdentity{
			PrimaryIdentifier: identifier,
			Type:              c.determineIdentifierType(identifier),
			RelatedIdentities: make(map[string][]string),
			Sources:           sources,
			Confidence:        float64(len(sources)) / float64(len(searchResult.ModuleResults)),
		}

		c.findRelatedIdentities(identity, searchResult)
		
		if identity.Confidence >= c.threshold {
			identities[identifier] = identity
		}
	}

	return c.rankIdentities(identities)
}

func (c *IdentityCorrelator) determineIdentifierType(identifier string) string {
	if strings.Contains(identifier, "@") {
		return "email"
	}
	if strings.Contains(identifier, ".") && !strings.Contains(identifier, " ") {
		return "domain"
	}
	return "username"
}

func (c *IdentityCorrelator) findRelatedIdentities(identity *CorrelatedIdentity, searchResult *ComprehensiveSearchResult) {
	for module, moduleResult := range searchResult.ModuleResults {
		if moduleResult.Error != nil {
			continue
		}

		for _, result := range moduleResult.Results {
			switch v := result.(type) {
			case map[string]interface{}:
				c.extractRelatedFromMap(identity, v, module)
			case string:
				if c.isRelated(identity.PrimaryIdentifier, v) {
					identity.RelatedIdentities[module] = append(identity.RelatedIdentities[module], v)
				}
			}
		}
	}
}

func (c *IdentityCorrelator) extractRelatedFromMap(identity *CorrelatedIdentity, data map[string]interface{}, source string) {
	fields := []string{"email", "username", "login", "name", "domain", "url"}
	
	for _, field := range fields {
		if value, ok := data[field].(string); ok && value != "" {
			if c.isRelated(identity.PrimaryIdentifier, value) || strings.Contains(value, identity.PrimaryIdentifier) {
				identity.RelatedIdentities[source] = append(identity.RelatedIdentities[source], value)
			}
		}
	}
}

func (c *IdentityCorrelator) isRelated(primary, candidate string) bool {
	primary = strings.ToLower(primary)
	candidate = strings.ToLower(candidate)

	if primary == candidate {
		return true
	}

	if strings.Contains(candidate, primary) || strings.Contains(primary, candidate) {
		return true
	}

	primaryParts := c.tokenize(primary)
	candidateParts := c.tokenize(candidate)

	matchCount := 0
	for _, pp := range primaryParts {
		for _, cp := range candidateParts {
			if pp == cp && len(pp) > 2 {
				matchCount++
			}
		}
	}

	return float64(matchCount) / float64(len(primaryParts)) >= 0.5
}

func (c *IdentityCorrelator) tokenize(s string) []string {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer(
		".", " ",
		"-", " ",
		"_", " ",
		"@", " ",
	)
	s = replacer.Replace(s)
	
	parts := strings.Fields(s)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 1 {
			filtered = append(filtered, part)
		}
	}
	
	return filtered
}

func (c *IdentityCorrelator) rankIdentities(identities map[string]*CorrelatedIdentity) []CorrelatedIdentity {
	ranked := make([]CorrelatedIdentity, 0, len(identities))
	
	for _, identity := range identities {
		totalRelated := 0
		for _, related := range identity.RelatedIdentities {
			totalRelated += len(related)
		}
		
		identity.Confidence = (identity.Confidence + float64(totalRelated)*0.1) / 2
		if identity.Confidence > 1.0 {
			identity.Confidence = 1.0
		}
		
		ranked = append(ranked, *identity)
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Confidence > ranked[j].Confidence
	})

	return ranked
}

type CrossReferencer struct {
	correlator *IdentityCorrelator
}

func NewCrossReferencer() *CrossReferencer {
	return &CrossReferencer{
		correlator: NewIdentityCorrelator(),
	}
}

func (r *CrossReferencer) CrossReference(results []*ComprehensiveSearchResult) *CrossReferenceReport {
	report := &CrossReferenceReport{
		CommonIdentities:  make(map[string][]string),
		IdentityGraph:     make(map[string][]string),
		StrongConnections: make([]Connection, 0),
	}

	allIdentifiers := make(map[string]map[string]bool)

	for _, result := range results {
		for idType, ids := range result.Identifiers {
			if _, exists := allIdentifiers[idType]; !exists {
				allIdentifiers[idType] = make(map[string]bool)
			}
			for _, id := range ids {
				allIdentifiers[idType][strings.ToLower(id)] = true
			}
		}
	}

	for idType, idMap := range allIdentifiers {
		ids := make([]string, 0, len(idMap))
		for id := range idMap {
			ids = append(ids, id)
		}
		if len(ids) > 1 {
			report.CommonIdentities[idType] = ids
		}
	}

	r.buildIdentityGraph(report, results)
	r.findStrongConnections(report)

	return report
}

func (r *CrossReferencer) buildIdentityGraph(report *CrossReferenceReport, results []*ComprehensiveSearchResult) {
	for _, result := range results {
		correlations := r.correlator.CorrelateIdentities(result)
		
		for _, correlation := range correlations {
			if correlation.Confidence < 0.7 {
				continue
			}

			primary := correlation.PrimaryIdentifier
			for _, relatedList := range correlation.RelatedIdentities {
				for _, related := range relatedList {
					if related != primary {
						report.IdentityGraph[primary] = append(report.IdentityGraph[primary], related)
					}
				}
			}
		}
	}
}

func (r *CrossReferencer) findStrongConnections(report *CrossReferenceReport) {
	processed := make(map[string]bool)

	for identity1, related1 := range report.IdentityGraph {
		for identity2, related2 := range report.IdentityGraph {
			if identity1 >= identity2 {
				continue
			}

			key := fmt.Sprintf("%s:%s", identity1, identity2)
			if processed[key] {
				continue
			}
			processed[key] = true

			commonCount := r.countCommon(related1, related2)
			if commonCount >= 2 || r.areDirectlyConnected(identity1, identity2, report.IdentityGraph) {
				connection := Connection{
					Identity1:      identity1,
					Identity2:      identity2,
					Strength:       float64(commonCount) / float64(len(related1)+len(related2)),
					CommonElements: r.findCommon(related1, related2),
				}
				
				if connection.Strength > 1.0 {
					connection.Strength = 1.0
				}
				
				report.StrongConnections = append(report.StrongConnections, connection)
			}
		}
	}

	sort.Slice(report.StrongConnections, func(i, j int) bool {
		return report.StrongConnections[i].Strength > report.StrongConnections[j].Strength
	})
}

func (r *CrossReferencer) countCommon(list1, list2 []string) int {
	set1 := make(map[string]bool)
	for _, item := range list1 {
		set1[strings.ToLower(item)] = true
	}

	count := 0
	for _, item := range list2 {
		if set1[strings.ToLower(item)] {
			count++
		}
	}

	return count
}

func (r *CrossReferencer) findCommon(list1, list2 []string) []string {
	set1 := make(map[string]bool)
	for _, item := range list1 {
		set1[strings.ToLower(item)] = true
	}

	common := make([]string, 0)
	for _, item := range list2 {
		if set1[strings.ToLower(item)] {
			common = append(common, item)
		}
	}

	return common
}

func (r *CrossReferencer) areDirectlyConnected(id1, id2 string, graph map[string][]string) bool {
	id1Lower := strings.ToLower(id1)
	id2Lower := strings.ToLower(id2)

	if related, exists := graph[id1]; exists {
		for _, r := range related {
			if strings.ToLower(r) == id2Lower {
				return true
			}
		}
	}

	if related, exists := graph[id2]; exists {
		for _, r := range related {
			if strings.ToLower(r) == id1Lower {
				return true
			}
		}
	}

	return false
}

type CrossReferenceReport struct {
	CommonIdentities  map[string][]string
	IdentityGraph     map[string][]string
	StrongConnections []Connection
}

type Connection struct {
	Identity1      string
	Identity2      string
	Strength       float64
	CommonElements []string
}