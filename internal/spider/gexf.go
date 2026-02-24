package spider

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

type gexfFile struct {
	XMLName xml.Name  `xml:"gexf"`
	XMLNS   string    `xml:"xmlns,attr"`
	Version string    `xml:"version,attr"`
	Meta    gexfMeta  `xml:"meta"`
	Graph   gexfGraph `xml:"graph"`
}

type gexfMeta struct {
	LastModified string `xml:"lastmodifieddate,attr"`
	Creator      string `xml:"creator"`
	Description  string `xml:"description"`
}

type gexfGraph struct {
	DefaultEdgeType  string               `xml:"defaultedgetype,attr"`
	Mode             string               `xml:"mode,attr"`
	AttributeClasses []gexfAttributeClass `xml:"attributes"`
	Nodes            gexfNodes            `xml:"nodes"`
	Edges            gexfEdges            `xml:"edges"`
}

type gexfAttributeClass struct {
	Class      string          `xml:"class,attr"`
	Attributes []gexfAttribute `xml:"attribute"`
}

type gexfAttribute struct {
	ID    string `xml:"id,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

type gexfNodes struct {
	Nodes []gexfNode `xml:"node"`
}

type gexfNode struct {
	ID        string        `xml:"id,attr"`
	Label     string        `xml:"label,attr"`
	AttValues gexfAttValues `xml:"attvalues"`
}

type gexfAttValues struct {
	AttValues []gexfAttValue `xml:"attvalue"`
}

type gexfAttValue struct {
	For   string `xml:"for,attr"`
	Value string `xml:"value,attr"`
}

type gexfEdges struct {
	Edges []gexfEdge `xml:"edge"`
}

type gexfEdge struct {
	ID        string        `xml:"id,attr"`
	Source    string        `xml:"source,attr"`
	Target   string        `xml:"target,attr"`
	Weight   string        `xml:"weight,attr,omitempty"`
	AttValues gexfAttValues `xml:"attvalues"`
}

func WriteGEXF(w io.Writer, graph *Graph, seedUser string) error {
	graph.mu.RLock()
	defer graph.mu.RUnlock()

	nodes := make([]gexfNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		label := node.Login
		if node.Name != "" {
			label = node.Name + " (" + node.Login + ")"
		}

		attValues := []gexfAttValue{
			{For: "0", Value: fmt.Sprintf("%d", node.Followers)},
			{For: "1", Value: fmt.Sprintf("%d", node.PublicRepos)},
			{For: "2", Value: node.Company},
			{For: "3", Value: node.Location},
			{For: "4", Value: fmt.Sprintf("%d", node.Depth)},
		}

		nodes = append(nodes, gexfNode{
			ID:        node.Login,
			Label:     label,
			AttValues: gexfAttValues{AttValues: attValues},
		})
	}

	edges := make([]gexfEdge, 0, len(graph.Edges))
	edgeID := 0
	for _, edge := range graph.Edges {
		attValues := []gexfAttValue{
			{For: "0", Value: edge.Type},
			{For: "1", Value: fmt.Sprintf("%d", edge.Weight)},
			{For: "2", Value: edge.Repo},
		}

		edges = append(edges, gexfEdge{
			ID:        fmt.Sprintf("e%d", edgeID),
			Source:    edge.Source,
			Target:   edge.Target,
			Weight:   fmt.Sprintf("%d", edge.Weight),
			AttValues: gexfAttValues{AttValues: attValues},
		})
		edgeID++
	}

	doc := gexfFile{
		XMLNS:   "http://gexf.net/1.3",
		Version: "1.3",
		Meta: gexfMeta{
			LastModified: time.Now().Format("2006-01-02"),
			Creator:      "gitslurp",
			Description:  fmt.Sprintf("Social graph for %s", seedUser),
		},
		Graph: gexfGraph{
			DefaultEdgeType: "directed",
			Mode:            "static",
			AttributeClasses: []gexfAttributeClass{
				{
					Class: "node",
					Attributes: []gexfAttribute{
						{ID: "0", Title: "followers", Type: "integer"},
						{ID: "1", Title: "public_repos", Type: "integer"},
						{ID: "2", Title: "company", Type: "string"},
						{ID: "3", Title: "location", Type: "string"},
						{ID: "4", Title: "depth", Type: "integer"},
					},
				},
				{
					Class: "edge",
					Attributes: []gexfAttribute{
						{ID: "0", Title: "type", Type: "string"},
						{ID: "1", Title: "weight", Type: "integer"},
						{ID: "2", Title: "repo", Type: "string"},
					},
				},
			},
			Nodes: gexfNodes{Nodes: nodes},
			Edges: gexfEdges{Edges: edges},
		},
	}

	w.Write([]byte(xml.Header))
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	return encoder.Encode(doc)
}
