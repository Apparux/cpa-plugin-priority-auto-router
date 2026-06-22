package main

import "testing"

func TestSortCandidatesPriorityDescending(t *testing.T) {
	got := sortCandidates([]Candidate{
		{Name: "low", ChannelType: channelTypeClaudeAPIKey, Priority: 10},
		{Name: "high", ChannelType: channelTypeClaudeAPIKey, Priority: 20},
	})
	if got[0].Name != "high" || got[1].Name != "low" {
		t.Fatalf("order = %v", candidateNames(got))
	}
}

func TestSortCandidatesCodexOAuthTieBreak(t *testing.T) {
	got := sortCandidates([]Candidate{
		{Name: "api", ChannelType: channelTypeClaudeAPIKey, Priority: 100},
		{Name: "oauth", ChannelType: channelTypeCodexOAuth, Priority: 100},
	})
	if got[0].Name != "oauth" || got[1].Name != "api" {
		t.Fatalf("order = %v", candidateNames(got))
	}
}

func TestSortCandidatesPreservesOriginalOrderForEqualNonOAuth(t *testing.T) {
	got := sortCandidates([]Candidate{
		{Name: "first", ChannelType: channelTypeClaudeAPIKey, Priority: 100},
		{Name: "second", ChannelType: channelTypeCodexAPIKey, Priority: 100},
	})
	if got[0].Name != "first" || got[1].Name != "second" {
		t.Fatalf("order = %v", candidateNames(got))
	}
}

func TestSortCandidatesPreservesOriginalOrderForEqualOAuth(t *testing.T) {
	got := sortCandidates([]Candidate{
		{Name: "first", ChannelType: channelTypeCodexOAuth, Priority: 100},
		{Name: "second", ChannelType: channelTypeCodexOAuth, Priority: 100},
	})
	if got[0].Name != "first" || got[1].Name != "second" {
		t.Fatalf("order = %v", candidateNames(got))
	}
}

func candidateNames(candidates []Candidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.Name)
	}
	return out
}
